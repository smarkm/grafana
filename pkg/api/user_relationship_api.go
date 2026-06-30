package api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/grafana/grafana/pkg/api/response"
	"github.com/grafana/grafana/pkg/infra/db"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/services/accesscontrol"
	"github.com/grafana/grafana/pkg/services/authn"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/user"
	"github.com/grafana/grafana/pkg/services/userrelationship"
	"github.com/grafana/grafana/pkg/web"
)

type userRelationshipSession struct {
	SuperId     string
	CustomerIds string
}

var (
	userRelationshipMap   = map[string]*userRelationshipSession{}
	userRelationshipMapMu sync.RWMutex
)

func getUserRelationshipSession(key string) *userRelationshipSession {
	userRelationshipMapMu.RLock()
	defer userRelationshipMapMu.RUnlock()
	return userRelationshipMap[key]
}

func setUserRelationshipSession(key string, session *userRelationshipSession) {
	userRelationshipMapMu.Lock()
	defer userRelationshipMapMu.Unlock()
	userRelationshipMap[key] = session
}

func deleteUserRelationshipSession(key string) {
	userRelationshipMapMu.Lock()
	defer userRelationshipMapMu.Unlock()
	delete(userRelationshipMap, key)
}

func (hs *HTTPServer) sessionKey(c *contextmodel.ReqContext) string {
	if cookie := c.GetCookie(hs.Cfg.LoginCookieName); cookie != "" {
		return cookie
	}
	if c.UserToken != nil {
		return c.UserToken.UnhashedToken
	}
	return ""
}

func cloneUserRelationshipSession(s *userRelationshipSession) *userRelationshipSession {
	if s == nil {
		return nil
	}
	return &userRelationshipSession{
		SuperId:     s.SuperId,
		CustomerIds: s.CustomerIds,
	}
}

func (hs *HTTPServer) bindCustomerIDs(ctx context.Context, login string, originalSessionKey string, cookieValue string) {
	if cookieValue == "" {
		return
	}

	superID := login
	customerIds := ""
	if originalSessionKey != "" {
		if entry := getUserRelationshipSession(originalSessionKey); entry != nil {
			superID = entry.SuperId
			customerIds = entry.CustomerIds
		}
	}

	if customerIds == "" {
		customerIds = hs.loadCustomerIds(ctx, superID)
	}

	setUserRelationshipSession(cookieValue, &userRelationshipSession{
		SuperId:     superID,
		CustomerIds: customerIds,
	})
	if customerIds != "" {
		hs.log.Info("bind session superId", "superId", superID, "customerIds", len(customerIds))
	}
}

func (hs *HTTPServer) loadCustomerIds(ctx context.Context, superID string) string {
	rel, err := hs.getUserRelationshipBySuperID(ctx, superID)
	if err != nil {
		if !errors.Is(err, userrelationship.ErrNotFound) {
			hs.log.Warn("Failed to query user relationship during bind", "superId", superID, "error", err)
		}
		return ""
	}

	if strings.EqualFold(rel.CustomerIds, "all") {
		return hs.expandAllCustomerIds(ctx)
	}
	return rel.CustomerIds
}

func (hs *HTTPServer) expandAllCustomerIds(ctx context.Context) string {
	search := &user.SearchUsersQuery{
		SignedInUser: &user.SignedInUser{
			IsGrafanaAdmin: true,
			Permissions: map[int64]map[string][]string{
				accesscontrol.GlobalOrgID: {accesscontrol.ActionUsersRead: {accesscontrol.ScopeGlobalUsersAll}},
			},
		},
	}

	result, err := hs.userService.Search(ctx, search)
	if err != nil {
		hs.log.Warn("Failed to expand all customer ids", "error", err)
		return ""
	}

	var customerIds strings.Builder
	for _, usr := range result.Users {
		if usr.Login == "admin" {
			continue
		}
		customerIds.WriteString(usr.Login)
		customerIds.WriteByte(',')
	}
	return customerIds.String()
}

func (hs *HTTPServer) ensureUserRelationshipSession(c *contextmodel.ReqContext) *userRelationshipSession {
	key := hs.sessionKey(c)
	if key == "" {
		return nil
	}

	if session := getUserRelationshipSession(key); session != nil {
		return session
	}

	if !c.IsSignedIn || c.SignedInUser == nil {
		return nil
	}

	hs.bindCustomerIDs(c.Req.Context(), c.SignedInUser.Login, "", key)
	return getUserRelationshipSession(key)
}

func (hs *HTTPServer) completeLoginWithUserRelationship(c *contextmodel.ReqContext, identity *authn.Identity) *response.NormalResponse {
	originalSessionKey := hs.sessionKey(c)
	resp := authn.HandleLoginResponse(c.Req, c.Resp, hs.Cfg, identity, hs.ValidateRedirectTo, hs.Features)
	if identity.SessionToken != nil {
		hs.bindCustomerIDs(c.Req.Context(), identity.Login, originalSessionKey, identity.SessionToken.UnhashedToken)
	}
	return resp
}

func (hs *HTTPServer) getUserRelationshipBySuperID(ctx context.Context, superID string) (*userrelationship.UserRelationship, error) {
	var rel userrelationship.UserRelationship
	err := hs.SQLStore.WithDbSession(ctx, func(sess *db.Session) error {
		has, err := sess.Table("user_relationship").
			Where("super_id = ?", superID).
			Cols("super_id", "customer_ids").
			Get(&rel)
		if err != nil {
			return err
		}
		if !has {
			return userrelationship.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &rel, nil
}

func (hs *HTTPServer) saveUserRelationship(ctx context.Context, rel userrelationship.UserRelationship) error {
	return hs.SQLStore.WithDbSession(ctx, func(sess *db.Session) error {
		_, err := sess.Insert(rel)
		return err
	})
}

func (hs *HTTPServer) updateUserRelationship(ctx context.Context, rel userrelationship.UserRelationship) error {
	return hs.SQLStore.WithDbSession(ctx, func(sess *db.Session) error {
		_, err := sess.Where("super_id = ?", rel.SuperId).Update(rel)
		return err
	})
}

func (hs *HTTPServer) deleteUserRelationship(ctx context.Context, superID string) error {
	return hs.SQLStore.WithDbSession(ctx, func(sess *db.Session) error {
		_, err := sess.Where("super_id = ?", superID).Delete(&userrelationship.UserRelationship{})
		return err
	})
}

func (hs *HTTPServer) queryAllUserRelationships(ctx context.Context) ([]userrelationship.UserRelationship, error) {
	var relationships []userrelationship.UserRelationship
	err := hs.SQLStore.WithDbSession(ctx, func(sess *db.Session) error {
		return sess.Find(&relationships)
	})
	return relationships, err
}

func (hs *HTTPServer) ImportUserRelastionShipData(c *contextmodel.ReqContext) response.Response {
	file, _, err := c.Req.FormFile("file")
	if err != nil {
		hs.log.Error("Failed to read file", "error", err)
		return response.Error(http.StatusBadRequest, "Failed to read file", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ",") {
			return response.Error(http.StatusBadRequest, "There items not contians comma, please check your file", nil)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		hs.log.Error("Failed to read file", "error", err)
	}

	count := 0
	ctx := c.Req.Context()
	for _, line := range lines {
		index := strings.Index(line, ",")
		superID := line[:index]
		customerIds := strings.ReplaceAll(line[index+1:], "\n", "")
		customerIds = strings.ReplaceAll(customerIds, " ", "")
		customerIds = strings.ReplaceAll(customerIds, "\"", "")

		if strings.Contains(line, "User ID") {
			continue
		}
		count++

		rel := userrelationship.UserRelationship{
			SuperId:     superID,
			CustomerIds: customerIds,
		}

		if _, err := hs.getUserRelationshipBySuperID(ctx, rel.SuperId); err != nil {
			if err := hs.saveUserRelationship(ctx, rel); err != nil {
				hs.log.Error("Failed save user relationship", "login", c.Login, "superId", rel.SuperId, "customerIds", rel.CustomerIds, "error", err)
				return response.Error(http.StatusInternalServerError, "Failed to save user relationship", err)
			}
			hs.log.Info("Save user relationship", "login", c.Login, "superId", rel.SuperId, "customerIds", rel.CustomerIds)
		} else {
			if err := hs.updateUserRelationship(ctx, rel); err != nil {
				hs.log.Error("Failed update user relationship", "login", c.Login, "superId", rel.SuperId, "customerIds", rel.CustomerIds, "error", err)
				return response.Error(http.StatusInternalServerError, "Failed to save user relationship", err)
			}
			hs.log.Info("Update user relationship", "login", c.Login, "superId", rel.SuperId, "customerIds", rel.CustomerIds)
		}
	}

	msg := fmt.Sprintf("Successful update total:%d user relationships by login:%s", count, c.Login)
	hs.log.Info(msg)
	return response.JSON(http.StatusOK, msg)
}

func (hs *HTTPServer) SaveUserRelationshipHandler(c *contextmodel.ReqContext) response.Response {
	var data userrelationship.UserRelationship
	if err := web.Bind(c.Req, &data); err != nil {
		return response.Error(http.StatusBadRequest, "bad request data", err)
	}

	if data.SuperId == "" {
		return response.Error(http.StatusBadRequest, "superId and customerIds are required", nil)
	}
	data.CustomerIds = strings.ReplaceAll(data.CustomerIds, " ", "")

	if err := hs.saveUserRelationship(c.Req.Context(), data); err != nil {
		hs.log.Error("Failed to save user relationship", "error", err)
		return response.Error(http.StatusInternalServerError, "Failed to save user relationship", err)
	}

	return response.JSON(http.StatusOK, "User relationship saved successfully")
}

func (hs *HTTPServer) UpdateUserRelationshipHandler(c *contextmodel.ReqContext) response.Response {
	var data userrelationship.UserRelationship
	if err := web.Bind(c.Req, &data); err != nil {
		return response.Error(http.StatusBadRequest, "bad request data", err)
	}

	if data.SuperId == "" {
		return response.Error(http.StatusBadRequest, "superID and subID are required", nil)
	}
	data.CustomerIds = strings.ReplaceAll(data.CustomerIds, " ", "")

	if err := hs.updateUserRelationship(c.Req.Context(), data); err != nil {
		return response.Error(http.StatusInternalServerError, "Failed to update user relationship", err)
	}

	return response.JSON(http.StatusOK, "User relationship updated successfully")
}

func (hs *HTTPServer) DeleteUserRelationshipHandler(c *contextmodel.ReqContext) response.Response {
	superID := web.Params(c.Req)[":superId"]
	if superID == "" {
		return response.Error(http.StatusBadRequest, "superId is required", nil)
	}

	if err := hs.deleteUserRelationship(c.Req.Context(), superID); err != nil {
		return response.Error(http.StatusInternalServerError, "Failed to delete user relationship", err)
	}

	return response.JSON(http.StatusOK, "User relationship deleted successfully")
}

func (hs *HTTPServer) QueryAllUserRelationshipsHandler(c *contextmodel.ReqContext) response.Response {
	relationships, err := hs.queryAllUserRelationships(c.Req.Context())
	if err != nil {
		return response.Error(http.StatusInternalServerError, "Failed to query user relationships", err)
	}

	return response.JSON(http.StatusOK, relationships)
}

func (hs *HTTPServer) QueryUserRelationshipBySuperIDHandler(c *contextmodel.ReqContext) response.Response {
	if !c.IsSignedIn || c.SignedInUser == nil {
		return response.Error(http.StatusUnauthorized, "Session expired", nil)
	}

	session := hs.ensureUserRelationshipSession(c)
	if session != nil {
		return response.JSON(http.StatusOK, userrelationship.UserRelationship{
			SuperId:     session.SuperId,
			CustomerIds: session.CustomerIds,
		})
	}

	superID := c.SignedInUser.Login
	rel, err := hs.getUserRelationshipBySuperID(c.Req.Context(), superID)
	if err != nil {
		if errors.Is(err, userrelationship.ErrNotFound) {
			return response.JSON(http.StatusOK, userrelationship.UserRelationship{})
		}
		return response.Error(http.StatusInternalServerError, "Failed to query user relationship", err)
	}

	if strings.EqualFold(rel.CustomerIds, "all") {
		rel.CustomerIds = hs.expandAllCustomerIds(c.Req.Context())
	}

	return response.JSON(http.StatusOK, rel)
}

func (hs *HTTPServer) SwitchUser(c *contextmodel.ReqContext) response.Response {
	userID := web.Params(c.Req)[":userId"]
	loggedInID := c.SignedInUser.Login

	oldKey := hs.sessionKey(c)
	oldSession := cloneUserRelationshipSession(getUserRelationshipSession(oldKey))

	targetUser, err := hs.userService.GetByLogin(c.Req.Context(), &user.GetUserByLoginQuery{LoginOrEmail: userID})
	if err != nil {
		hs.log.Error("Failed to switch user", "error", err, "userId", userID)
		return response.Error(http.StatusInternalServerError, "Failed to switch user", err)
	}

	if oldSession != nil {
		hs.log.Info("Switch user superId", "superId", oldSession.SuperId, "from", loggedInID, "to", userID)
	}

	if err := hs.loginUserWithUser(targetUser, c); err != nil {
		return response.Error(http.StatusInternalServerError, "Error while signing in user", err)
	}

	newKey := hs.sessionKey(c)
	if oldSession != nil && newKey != "" {
		setUserRelationshipSession(newKey, oldSession)
	}
	deleteUserRelationshipSession(oldKey)

	metrics.MApiLoginPost.Inc()
	return response.JSON(http.StatusOK, map[string]any{"message": "Logged in"})
}
