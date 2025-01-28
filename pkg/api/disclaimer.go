package api

import (
	"fmt"
	"log"
	"os"

	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/setting"
)

func (hs *HTTPServer) GetDisclaimer(c *models.ReqContext) Response {
	var rs = ""
	fileName := setting.HomePath + "/conf/disclaimer.txt"
	content, err := os.ReadFile(fileName)
	if err != nil {
		// 忽略错误，不终止程序
		log.Printf("Warning: failed to read file: %v", err)
	} else {
		rs = string(content)
		fmt.Println(rs) // 假设您要对读取的内容进行某些操作
	}
	return Success(rs)
}
