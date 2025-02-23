#!/bin/bash
#go env -w GOPROXY=https://goproxy.cn,direct
go env
cd /grafana && make build-server