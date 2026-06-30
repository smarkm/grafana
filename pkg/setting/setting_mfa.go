package setting

import "time"

type AuthMFAEmailOTPSettings struct {
	Enabled        bool
	CodeExpiration time.Duration
}

func (cfg *Cfg) readMFAEmailOTPSettings() {
	section := cfg.SectionWithEnvOverrides("auth.mfa_email_otp")
	cfg.MFAEmailOTP = AuthMFAEmailOTPSettings{
		Enabled:        section.Key("enabled").MustBool(false),
		CodeExpiration: section.Key("code_expiration").MustDuration(5 * time.Minute),
	}
}
