## Base code info
1. This code base tag v7.3.7, can show the log with `git log` for update
2. The customized version will in branch v7.3.7o for oss3ra

## Update list
1. `login_max_attempts` under `security` section support max login attem config, default is 5 and will `disable user` if got max attempts invalid password
2. `login_with_otp = true` under `users` section support OTP with email
    `ossera_email_otp_title`  under `smtp` section the OTP msg title
    `ossera_email_otp_body` the OTP msg body, use `%s` msg template will replace otp code, like "Wellcom, you otp code is :%s"

## Dev
### OTP support
1. If enable otp use the config, system will auto response the main page
2. By default is still grafana index main page, for OTP will return l`oginOTP.html` that style can customized but when login will call the setting url to sendOTP
3.  Once otp send success will redirect to `otp.html` page, then verify OTP to login system 



## Build runnable server
1. `docker build -t dockergrafana -f ./MyDockerfile .` run this command to build the grafana builder env, this is customized for specific os, and if your local already have this build before, just `optional`
2. `docker run --rm -v /repo/grafana/grafana:/grafana -it dockergrafana /bin/sh`, this to start builder container, then run `cd /grafana && make build-server` to build grafana-server bin, `Note:` make sure mount your change workspace correctly

