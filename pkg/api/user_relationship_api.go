package api

import (
	"net/http"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/middleware"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/setting"
)

var userRelationshipMap = map[string]*models.UserRelationship{}

// Handler to save user relationship
func (hs *HTTPServer) SaveUserRelationshipHandler(c *models.ReqContext, data models.UserRelationship) Response {

	if data.SuperId == "" {
		return Error(http.StatusBadRequest, "superId and customerIds are required", nil)

	}

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
	log.Infof("superID:", c.Get("user"))
	if err := bus.Dispatch(query); err != nil {
		return Error(http.StatusInternalServerError, "Failed to query user relationships", nil)
	}

	return JSON(http.StatusOK, query.Result)
}

// Handler to query user relationships by superID
func (hs *HTTPServer) QueryUserRelationshipBySuperIDHandler(c *models.ReqContext) Response {
	cookie := c.GetCookie(setting.LoginCookieName)
	superID := userRelationshipMap[cookie].SuperId
	if superID == "" {
		return JSON(http.StatusOK, "")
	}

	query := &models.QueryUserRelationshipBySuperIdQuery{SuperId: superID}
	if err := bus.Dispatch(query); err != nil {
		return Error(http.StatusInternalServerError, "Failed to query user relationship by superID", nil)
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
		if err := hs.ValidateRedirectTo(redirectTo); err == nil {
			result["redirectUrl"] = redirectTo
		} else {
			log.Infof("Ignored invalid redirect_to cookie value: %v", redirectTo)
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
