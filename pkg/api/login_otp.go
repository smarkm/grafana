package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	claims "github.com/grafana/authlib/types"
	"github.com/grafana/grafana/pkg/api/response"
	"github.com/grafana/grafana/pkg/apimachinery/errutil"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/infra/remotecache"
	"github.com/grafana/grafana/pkg/services/auth"
	"github.com/grafana/grafana/pkg/services/authn"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/notifications"
	"github.com/grafana/grafana/pkg/services/user"
	"github.com/grafana/grafana/pkg/util"
	"github.com/grafana/grafana/pkg/web"
)

const loginMFAOTPKeyPrefix = "login-mfa-otp-%s"

var (
	errMFAOTPInvalidCode     = errutil.Unauthorized("mfa-otp.invalid.code", errutil.WithPublicMessage("Invalid verification code"))
	errMFAOTPExpired         = errutil.Unauthorized("mfa-otp.expired", errutil.WithPublicMessage("Verification code has expired"))
	errMFAOTPMissingToken    = errutil.BadRequest("mfa-otp.missing.token", errutil.WithPublicMessage("Missing verification token"))
	errMFAOTPMissingCode     = errutil.BadRequest("mfa-otp.missing.code", errutil.WithPublicMessage("Missing verification code"))
	errMFAOTPInternal        = errutil.Internal("mfa-otp.failed", errutil.WithPublicMessage("An internal error occurred during MFA verification"))
	errMFAOTPEmailNotSent    = errutil.Internal("mfa-otp.email-failed", errutil.WithPublicMessage("Failed to send verification email"))
	errMFAOTPNoEmail         = errutil.BadRequest("mfa-otp.no-email", errutil.WithPublicMessage("User does not have an email address configured"))
	errMFAOTPSMTPNotEnabled  = errutil.Internal("mfa-otp.smtp-disabled", errutil.WithPublicMessage("Email is not configured"))
)

type mfaOTPVerifyForm struct {
	OTPToken string `json:"otpToken" binding:"Required"`
	OTPCode  string `json:"otpCode" binding:"Required"`
}

type mfaOTPIdentityCache struct {
	ID              string `json:"id"`
	OrgID           int64  `json:"org_id"`
	AuthenticatedBy string `json:"authenticated_by"`
	Login           string `json:"login"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	OTPCode         string `json:"otp_code"`
}

func (hs *HTTPServer) loginPostWithMFA(c *contextmodel.ReqContext) response.Response {
	if !hs.Cfg.Smtp.Enabled {
		return response.Err(errMFAOTPSMTPNotEnabled)
	}

	identity, err := hs.authnService.AuthenticateClient(c.Req.Context(), authn.ClientForm, &authn.Request{HTTPRequest: c.Req})
	if err != nil {
		tokenErr := &auth.CreateTokenErr{}
		if errors.As(err, &tokenErr) {
			return response.Error(tokenErr.StatusCode, tokenErr.ExternalErr, tokenErr.InternalErr)
		}
		return response.Err(err)
	}

	email := identity.Email
	if email == "" {
		userID, idErr := identity.GetInternalID()
		if idErr != nil {
			return response.Err(idErr)
		}
		usr, usrErr := hs.userService.GetByID(c.Req.Context(), &user.GetUserByIDQuery{ID: userID})
		if usrErr != nil {
			return response.Err(usrErr)
		}
		email = usr.Email
	}

	if email == "" {
		return response.Err(errMFAOTPNoEmail)
	}

	otpToken, otpCode, err := hs.startMFAOTP(c.Req.Context(), identity, email)
	if err != nil {
		return response.Err(err)
	}
	_ = otpCode

	metrics.MApiLoginPost.Inc()
	return response.JSON(http.StatusOK, util.DynMap{
		"message":     "OTP required",
		"otpRequired": true,
		"otpToken":    otpToken,
		"email":       maskEmail(email),
	})
}

func (hs *HTTPServer) LoginOTPVerify(c *contextmodel.ReqContext) response.Response {
	form := mfaOTPVerifyForm{}
	if err := web.Bind(c.Req, &form); err != nil {
		return response.Err(errMFAOTPMissingToken.Errorf("failed to parse request: %w", err))
	}

	if form.OTPToken == "" {
		return response.Err(errMFAOTPMissingToken)
	}
	if form.OTPCode == "" {
		return response.Err(errMFAOTPMissingCode)
	}

	cacheKey := fmt.Sprintf(loginMFAOTPKeyPrefix, form.OTPToken)
	jsonData, err := hs.RemoteCacheService.Get(c.Req.Context(), cacheKey)
	if err != nil {
		if errors.Is(err, remotecache.ErrCacheItemNotFound) {
			return response.Err(errMFAOTPExpired)
		}
		return response.Err(errMFAOTPInternal.Errorf("cache error: %w", err))
	}

	var entry mfaOTPIdentityCache
	if err := json.Unmarshal(jsonData, &entry); err != nil {
		return response.Err(errMFAOTPInternal.Errorf("failed to parse MFA cache entry: %w", err))
	}

	if subtle.ConstantTimeCompare([]byte(entry.OTPCode), []byte(form.OTPCode)) != 1 {
		return response.Err(errMFAOTPInvalidCode)
	}

	if err := hs.RemoteCacheService.Delete(c.Req.Context(), cacheKey); err != nil {
		hs.log.Warn("failed to delete MFA OTP cache entry", "err", err)
	}

	identity := &authn.Identity{
		ID:              entry.ID,
		Type:            claims.TypeUser,
		OrgID:           entry.OrgID,
		Login:           entry.Login,
		Email:           entry.Email,
		Name:            entry.Name,
		AuthenticatedBy: entry.AuthenticatedBy,
		ClientParams:    authn.ClientParams{FetchSyncedUser: true, SyncPermissions: true},
	}

	authnReq := &authn.Request{HTTPRequest: c.Req}
	identity, err = hs.authnService.CreateLoginSession(c.Req.Context(), identity, authnReq)
	if err != nil {
		tokenErr := &auth.CreateTokenErr{}
		if errors.As(err, &tokenErr) {
			return response.Error(tokenErr.StatusCode, tokenErr.ExternalErr, tokenErr.InternalErr)
		}
		return response.Err(err)
	}

	metrics.MApiLoginPost.Inc()
	return hs.completeLoginWithUserRelationship(c, identity)
}

func (hs *HTTPServer) startMFAOTP(ctx context.Context, identity *authn.Identity, email string) (string, string, error) {
	otpCode, err := util.GetRandomString(6, []byte("0123456789")...)
	if err != nil {
		return "", "", errMFAOTPInternal.Errorf("failed to generate OTP code: %w", err)
	}

	otpToken, err := util.GetRandomString(32)
	if err != nil {
		return "", "", errMFAOTPInternal.Errorf("failed to generate OTP token: %w", err)
	}

	emailCmd := notifications.SendEmailCommand{
		To:       []string{email},
		Template: "login_otp_mfa",
		Data: map[string]any{
			"Email":  email,
			"OTPCode": otpCode,
			"Expire": int(hs.Cfg.MFAEmailOTP.CodeExpiration.Minutes()),
		},
	}

	if err := hs.NotificationService.SendEmailCommandHandlerSync(ctx, &notifications.SendEmailCommandSync{
		SendEmailCommand: emailCmd,
	}); err != nil {
		return "", "", errMFAOTPEmailNotSent.Errorf("failed to send MFA OTP email: %w", err)
	}

	entry := mfaOTPIdentityCache{
		ID:              identity.ID,
		OrgID:           identity.OrgID,
		AuthenticatedBy: identity.AuthenticatedBy,
		Login:           identity.Login,
		Email:           email,
		Name:            identity.Name,
		OTPCode:         otpCode,
	}

	valueBytes, err := json.Marshal(entry)
	if err != nil {
		return "", "", err
	}

	cacheKey := fmt.Sprintf(loginMFAOTPKeyPrefix, otpToken)
	if err := hs.RemoteCacheService.Set(ctx, cacheKey, valueBytes, hs.Cfg.MFAEmailOTP.CodeExpiration); err != nil {
		return "", "", errMFAOTPInternal.Errorf("cache error: %w", err)
	}

	return otpToken, otpCode, nil
}

func maskEmail(email string) string {
	at := -1
	for i, c := range email {
		if c == '@' {
			at = i
			break
		}
	}
	if at <= 1 {
		return email
	}
	return email[:1] + "***" + email[at:]
}
