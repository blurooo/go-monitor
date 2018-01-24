package monitor

import (
	"strconv"
	"os"
	"bytes"
)

// 默认告警处理方式
func defaultAlert(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData) {
	alertTypeString := "未知"
	if alertType == SLOW {
		alertTypeString = "时延达标率"
	} else if alertType == FAIL {
		alertTypeString = "访问成功率"
	}
	var alertString bytes.Buffer
	alertString.WriteString("\n 告警：\n   客户端上报类型：" + clientName + "\n   接口：" + interfaceName + "\n   告警类型：" + alertTypeString + "\n   最近" + strconv.Itoa(len(recentOutputData)) + "状态：")
	for i, r := range recentOutputData {
		var rate float64
		if alertType == SLOW {
			rate = r.FastRate
		} else if alertType == FAIL {
			rate = r.SuccessRate
		}
		alertString.WriteString("\n     " + strconv.Itoa(i + 1) + ". " + "调用" + strconv.FormatUint(uint64(r.Count), 10) + "次，" + alertTypeString + "为" + strconv.FormatFloat(float64(rate * 100), 'f', 2, 64) + "%")
	}
	os.Stderr.WriteString(alertString.String() + "\n")
}

// 默认恢复通知处理方式
func defaultRecover(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData) {
	alertTypeString := "未知"
	if alertType == SLOW {
		alertTypeString = "时延达标率"
	} else if alertType == FAIL {
		alertTypeString = "访问成功率"
	}
	var alertString bytes.Buffer
	alertString.WriteString("\n 恢复通知：\n   客户端上报类型：" + clientName + "\n   接口：" + interfaceName + "\n   恢复类型：" + alertTypeString + "\n   最近" + strconv.Itoa(len(recentOutputData)) + "状态：")
	for i, r := range recentOutputData {
		var rate float64
		if alertType == SLOW {
			rate = r.FastRate
		} else if alertType == FAIL {
			rate = r.SuccessRate
		}
		alertString.WriteString("\n     " + strconv.Itoa(i + 1) + ". " + "调用" + strconv.FormatUint(uint64(r.Count), 10) + "次，" + alertTypeString + "为" + strconv.FormatFloat(float64(rate * 100), 'f', 2, 64) + "%")
	}
	os.Stderr.WriteString(alertString.String() + "\n")
}