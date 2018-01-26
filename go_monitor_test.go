package monitor

import (
	"testing"
	"time"
)


func Test1(t *testing.T) {
	alertTimes, _ := reportPipeline(FAIL, []bool {false, false, true, false})
	if alertTimes != 0 {
		t.Error("false false true false 失败", "告警次数", alertTimes)
	}
}


func Test2(t *testing.T) {
	alertTimes, recoverTimes := reportPipeline(FAIL, []bool {false, false, false, true, true, true})
	if alertTimes != 1 || recoverTimes != 1 {
		t.Error("false, false, false, true, true, true 失败", "告警次数", alertTimes, "恢复次数", recoverTimes)
	}
}

func Test3(t *testing.T) {
	alertTimes, _ := reportPipeline(FAIL, []bool {false, true, false, false})
	if alertTimes != 0 {
		t.Error("false true false false 失败", "告警次数", alertTimes)
	}
}

func Test4(t *testing.T) {
	alertTimes, recoverTimes := reportPipeline(FAIL, []bool {false, false, false, true, true, false, true, true, true})
	if alertTimes != 1 || recoverTimes != 1 {
		t.Error("false, false, false, true, true, true 失败", "告警次数", alertTimes, "恢复次数", recoverTimes)
	}
}

// 按设定的上报流水测试， 返回告警次数
func reportPipeline(alertType AlertType, pipeline []bool) (alertTimes int, recoverTimes int) {
	var ms uint32 = 0
	code := 0
	var testReportClient = Register(ReportClientConfig {
		Name: "告警测试",
		StatisticalCycle: 50,										// 上报统计周期50ms
		AlertCaller: func(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData) {
			alertTimes++
		},
		RecoverCaller: func(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData) {
			recoverTimes++
		},
	})
	for _, success := range pipeline {
		if success {
			ms = 1
			code = 200
		} else {
			if alertType == SLOW {
				ms = 1000
				code = 200
			} else if alertType == FAIL {
				code = 500
			}
		}
		testReportClient.Report("GET - 测试接口", ms, code)
		time.Sleep(60 * time.Millisecond)
	}
	return
}

func BenchmarkReport(b *testing.B) {
	testReportClient2 := Register(ReportClientConfig{
		Name:              "告警测试",
		StatisticalCycle:  2000,
		ChannelCacheCount: 0,
	})
	for i := 0; i < b.N; i++ {
		testReportClient2.Report("GET - 性能测试", uint32(i), 200)
	}
}