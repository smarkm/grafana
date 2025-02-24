package api

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/middleware"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/setting"
)

var userRelationshipMap = map[string]*models.UserRelationship{}

func (hs *HTTPServer) ImportUserRelastionShipData(c *models.ReqContext) Response {
	file, _, err := c.Req.FormFile("file")
	if err != nil {
		hs.log.Error("Failed to read file", err.Error())
		return Error(http.StatusBadRequest, "Failed to read file", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	// Slice to hold lines
	var lines []string

	// Read the file line by line
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ",") {
			return Error(http.StatusBadRequest, "There items not contians comma, please check your file", nil)
		}
		lines = append(lines, line)
	}

	// Check for errors during scanning
	if err := scanner.Err(); err != nil {
		hs.log.Error("Failed to read file", err.Error())
	}

	// Print the lines (or process them as needed)
	count := 0
	for _, line := range lines {

		index := strings.Index(line, ",")
		supperId := line[:index]
		customerIds := strings.ReplaceAll(line[index+1:], "\n", "")
		customerIds = strings.ReplaceAll(customerIds, " ", "")
		customerIds = strings.ReplaceAll(customerIds, "\"", "")
		fmt.Println(supperId + ":" + customerIds)
		if strings.Contains(line, "User ID") {
			continue
		}
		count++
		userRelationShip := &models.UserRelationship{
			SuperId:     supperId,
			CustomerIds: customerIds,
		}

		query := &models.QueryUserRelationshipBySuperIdQuery{SuperId: userRelationShip.SuperId}
		if err := bus.Dispatch(query); err != nil {
			save := &models.SaveUserRelationshipCommand{Data: *userRelationShip}
			if err := bus.Dispatch(save); err != nil {
				hs.log.Error("Failed save user relationship", "login", c.Login, "superId", userRelationShip.SuperId, "CustomerIds", userRelationShip.CustomerIds)
				return Error(http.StatusInternalServerError, "Failed to save user relationship", nil)
			} else {
				hs.log.Info("Save user relationship", "login", c.Login, "superId", userRelationShip.SuperId, "CustomerIds", userRelationShip.CustomerIds)
			}
		} else {
			update := &models.UpdateUserRelationshipCommand{Data: *userRelationShip}
			if err := bus.Dispatch(update); err != nil {
				hs.log.Error("Failed update user relationship", "login", c.Login, "superId", userRelationShip.SuperId, "CustomerIds", userRelationShip.CustomerIds)
				return Error(http.StatusInternalServerError, "Failed to save user relationship", nil)
			} else {
				hs.log.Info("Update user relationship", "login", c.Login, "superId", userRelationShip.SuperId, "CustomerIds", userRelationShip.CustomerIds)
			}
		}
	}
	msg := fmt.Sprintf("Successful update total:%s user relationships by login:%s", strconv.Itoa(count), c.Login)
	hs.log.Info(msg)
	return Success(msg)

}

func (hs *HTTPServer) ExportUserRelastionShipData(c *models.ReqContext) Response {
	fileName := c.Params(":filename")

	// 检查文件是否存在
	fileContent := []byte("")
	query := &models.QueryAllUserRelationshipsQuery{}
	log.Infof("superID:", c.Get("user"))
	if err := bus.Dispatch(query); err != nil {
		return Error(http.StatusInternalServerError, "Failed to query user relationships", nil)
	}

	for i := 0; i < len(query.Result); i++ {
		userRelationship := query.Result[i]
		fileContent = append(fileContent, []byte(fmt.Sprintf("%s,%v\n", userRelationship.SuperId, userRelationship.CustomerIds))...)
	}

	// 将文件内容写入响应体
	return Respond(http.StatusOK, fileContent).
		Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName)).
		Header("Content-Type", "application/octet-stream").
		Header("Content-Length", fmt.Sprintf("%d", len(fileContent)))
}

// Handler to save user relationship
func (hs *HTTPServer) SaveUserRelationshipHandler(c *models.ReqContext, data models.UserRelationship) Response {

	if data.SuperId == "" {
		return Error(http.StatusBadRequest, "superId and customerIds are required", nil)

	}
	data.CustomerIds = strings.ReplaceAll(data.CustomerIds, " ", "")
	cmd := &models.SaveUserRelationshipCommand{Data: data}
	if err := bus.Dispatch(cmd); err != nil {
		hs.log.Error("Failed to save user relationship", err.Error())
		return Error(http.StatusInternalServerError, "Failed to save user relationship", nil)
	}

	return Success("User relationship saved successfully")
}

// Handler to update user relationship
func (hs *HTTPServer) UpdateUserRelationshipHandler(c *models.ReqContext, data models.UserRelationship) Response {
	if data.SuperId == "" {
		return Error(http.StatusBadRequest, "superID and subID are required", nil)
	}
	data.CustomerIds = strings.ReplaceAll(data.CustomerIds, " ", "")
	cmd := &models.UpdateUserRelationshipCommand{Data: data}
	if err := bus.Dispatch(cmd); err != nil {
		return Error(http.StatusInternalServerError, "Failed to update user relationship", nil)
	}

	return Success("User relationship updated successfully")
}

// Handler to delete user relationship
func (hs *HTTPServer) DeleteUserRelationshipHandler(c *models.ReqContext) Response {
	superID := c.Params("superId")
	if superID == "" {
		return Error(http.StatusBadRequest, "superId is required", nil)
	}

	cmd := &models.DeleteUserRelationshipCommand{SuperId: superID}
	if err := bus.Dispatch(cmd); err != nil {
		return Error(http.StatusInternalServerError, "Failed to delete user relationship", nil)
	}

	return Success("User relationship deleted successfully")
}

// Handler to query all user relationships
func (hs *HTTPServer) QueryAllUserRelationshipsHandler(c *models.ReqContext) Response {
	query := &models.QueryAllUserRelationshipsQuery{}
	if err := bus.Dispatch(query); err != nil {
		return Error(http.StatusInternalServerError, "Failed to query user relationships", nil)
	}

	return JSON(http.StatusOK, query.Result)
}

// Handler to query user relationships by superID
func (hs *HTTPServer) QueryUserRelationshipBySuperIDHandler(c *models.ReqContext) Response {
	cookie := c.GetCookie(setting.LoginCookieName)
	ck := userRelationshipMap[cookie]
	if ck == nil {
		return JSON(http.StatusUnauthorized, "Session expired")
	}
	superID := ck.SuperId

	query := &models.QueryUserRelationshipBySuperIdQuery{SuperId: superID}
	if err := bus.Dispatch(query); err != nil {
		return JSON(http.StatusOK, models.UserRelationship{}) //no related record
	} else if strings.ToLower(query.Result.CustomerIds) == "all" {
		search := &models.SearchUsersQuery{}
		if err := bus.Dispatch(search); err != nil {
			hs.log.Error("Failed to search users", err.Error())
			return Error(http.StatusInternalServerError, "Failed to search users", nil)
		}
		query.Result.CustomerIds = ""
		for _, user := range search.Result.Users {
			if user.Login == "admin" {
				continue
			}
			query.Result.CustomerIds += user.Login + ","
		}
	}

	return JSON(http.StatusOK, query.Result)
}

func (hs *HTTPServer) BindCustomerIDs(c *models.ReqContext, user *models.User, orignalCookie string, cookieValue string) {
	superID := user.Login
	if orignalCookie != "" {
		superID = userRelationshipMap[orignalCookie].SuperId
	}
	query := &models.QueryUserRelationshipBySuperIdQuery{SuperId: superID}
	if err := bus.Dispatch(query); err != nil {
		userRelationshipMap[cookieValue] = &models.UserRelationship{SuperId: superID, CustomerIds: ""}
		return
	}
	customerIds := query.Result.CustomerIds
	if customerIds != "" {
		userRelationshipMap[cookieValue] = &models.UserRelationship{SuperId: superID, CustomerIds: customerIds}
		log.Infof("bind session superId: %v, customerIds: %v with ck: %v", superID, customerIds, cookieValue)
	}
}

func (hs *HTTPServer) SwitchUser(c *models.ReqContext) Response {
	userId := c.Params(":userId")
	var user *models.User
	var response *NormalResponse
	loginedID := c.SignedInUser.Login

	authQuery := &models.LoginUserQuery{
		ReqContext:     c,
		Username:       userId,
		IpAddress:      c.Req.RemoteAddr,
		NoPasswdVerify: true,
	}

	err := bus.Dispatch(authQuery)
	if err != nil {
		response = Error(http.StatusInternalServerError, "Failed to switch user", err)
		log.Errorf(1, "Failed to switch user: %v", err)
		return response
	}
	user = authQuery.User

	err = hs.loginUserWithUser(user, c)
	if err != nil {
		response = Error(http.StatusInternalServerError, "Error while signing in user", err)
		return response
	}

	result := map[string]interface{}{
		"message": "Logged in",
	}
	if redirectTo := c.GetCookie("redirect_to"); len(redirectTo) > 0 {
		if err := hs.ValidateRedirectTo(redirectTo); err != nil {
			// the user is already logged so instead of rendering the login page with error
			// it should be redirected to the home page.
			log.Debugf("Ignored invalid redirect_to cookie value: %v", redirectTo)
			result["redirect_to"] = hs.Cfg.AppSubUrl + "/"
		}
		middleware.DeleteCookie(c.Resp, "redirect_to", hs.CookieOptionsFromCfg)

	}
	ck := c.GetCookie(setting.LoginCookieName)
	superId := userRelationshipMap[ck].SuperId
	log.Infof("Switch user superId: %v, with ck:%v,from current login: %v to new login: %v", superId, ck, loginedID, userId)
	delete(userRelationshipMap, ck)
	metrics.MApiLoginPost.Inc()
	response = JSON(http.StatusOK, result)
	return response
}
