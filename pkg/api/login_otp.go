package api

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/grafana/pkg/api/dtos"
	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/login"
	"github.com/grafana/grafana/pkg/middleware"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/registry"
	"github.com/grafana/grafana/pkg/services/notifications"
	"github.com/grafana/grafana/pkg/setting"
)

type OTP struct {
	OTP      string    `json:"otp"`
	ExpireAt time.Time `json:"expireAt"`
	Code     string    `json:"code"`
	User     string    `json:"user"`
	Password string    `json:"password"`
}

var (
	otpMap = make(map[string]OTP)
)

func (hs *HTTPServer) LoginPostWithOTP(c *models.ReqContext, cmd dtos.LoginCommand) Response {
	authModule := ""
	var user *models.User
	var resp *NormalResponse

	if cmd.OTP != "" {
		otp, exist := otpMap[cmd.Code]
		if !exist {
			resp = Error(401, "Error while signing in user", nil)
			return resp
		}
		if otp.OTP != cmd.OTP {
			resp = Error(401, "Invalid OTP", nil)
			return resp

		}
		if otp.ExpireAt.Before(time.Now()) {
			resp = Error(401, "OTP expired", nil)
			delete(otpMap, cmd.Code)
			return resp
		}
		cmd.User = otp.User
		cmd.Password = otp.Password
	}

	defer func() {
		err := resp.err
		if err == nil && resp.errMessage != "" {
			err = errors.New(resp.errMessage)
		}
		hs.HooksService.RunLoginHook(&models.LoginInfo{
			AuthModule:    authModule,
			User:          user,
			LoginUsername: cmd.User,
			HTTPStatus:    resp.status,
			Error:         err,
		}, c)
	}()

	if setting.DisableLoginForm {
		resp = Error(http.StatusUnauthorized, "Login is disabled", nil)
		return resp
	}
	getUser := models.GetUserByLoginQuery{LoginOrEmail: cmd.User}
	err2 := bus.Dispatch(&getUser)
	if err2 != nil {
		resp = Error(http.StatusUnauthorized, "Failed to get user", nil)
		hs.log.Error("Failed to get user", "error", err2)
		return resp
	} else {
		if getUser.Result != nil {
			user = getUser.Result
		}
	}
	authQuery := &models.LoginUserQuery{
		ReqContext: c,
		Username:   cmd.User,
		Password:   cmd.Password,
		IpAddress:  c.Req.RemoteAddr,
	}

	err := bus.Dispatch(authQuery)
	authModule = authQuery.AuthModule
	if err != nil {
		if strings.HasPrefix(err.Error(), "Too many") {
			if user != nil {
				updateUserCmd := models.DisableUserCommand{UserId: user.Id, IsDisabled: true}
				err2 = bus.Dispatch(&updateUserCmd)
				if err2 != nil {
					hs.log.Error("Failed to disable user", "error", err)
				}
			}
			resp = Error(401, "Too many invalid password attemped, account is locked", err)
		} else {
			if errors.Is(err, login.ErrUserDisabled) {
				hs.log.Warn("User is disabled", "user", cmd.User)
				resp = Error(401, "User is disabled", err)
				return resp
			}
			resp = Error(401, "Invalid username or password", err)
		}
		if errors.Is(err, login.ErrInvalidCredentials) || errors.Is(err, login.ErrTooManyLoginAttempts) || errors.Is(err,
			models.ErrUserNotFound) {
			return resp
		}

		// Do not expose disabled status,
		// just show incorrect user credentials error (see #17947)
		if errors.Is(err, login.ErrUserDisabled) {
			hs.log.Warn("User is disabled", "user", cmd.User)
			return resp
		}

		resp = Error(500, "Error while trying to authenticate user", err)
		return resp
	}
	// redirect to OTP verify page
	if cmd.OTP == "" {
		code := uuid.New().String()
		otp := randomOTP()
		otpMap[code] = OTP{Code: code, OTP: otp, ExpireAt: time.Now().Add(2 * time.Minute), User: cmd.User, Password: cmd.Password}
		hs.log.Info("Send otp ...", "user", cmd.User, "code", code, "otp", otp, "email", user.Email)
		if user.Email == "" {
			hs.log.Error("No email information for send otp ", "user", user.Login)
		} else {
			if !sendOTP(hs, user, code, otp) {
				resp = Error(500, "Failed send OTP to user", err)
				return resp
			}
		}

		resp = Success("/public/otp.html?code=" + code)
		return resp
	}
	user = authQuery.User

	err = hs.loginUserWithUser(user, c)
	if err != nil {
		resp = Error(http.StatusInternalServerError, "Error while signing in user", err)
		return resp
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

	metrics.MApiLoginPost.Inc()
	resp = JSON(http.StatusOK, result)
	return resp
}

// rand.Seed(time.Now().UnixNano())randomNumber := rand.Intn(900000) + 100000
func randomOTP() string {
	rand.Seed(time.Now().UnixNano())
	randomNumber := rand.Intn(900000) + 100000
	return strconv.Itoa(randomNumber)
}

func sendOTP(hs *HTTPServer, user *models.User, code string, otp string) bool {
	ns := registry.GetService("NotificationService").Instance.(*notifications.NotificationService)
	to := []string{user.Email}
	subject := hs.Cfg.Smtp.OsseraEmailOtpTitle
	body := fmt.Sprintf(hs.Cfg.Smtp.OsseraEmailOtpBody, otp)
	msg := notifications.Message{To: to, Subject: subject, Body: body, From: hs.Cfg.Smtp.FromAddress}
	_, err := ns.SendEmail(&msg)
	if err != nil {
		hs.log.Error("Send otp failed ", user, user.Login, "code", code, "error", err)
		return false
	}
	hs.log.Info("Send otp success", "user", user.Login, "code", code)
	return true
}
