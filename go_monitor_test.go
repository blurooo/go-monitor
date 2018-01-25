package monitor

import (
	"testing"
	"time"
	"math/rand"
)


func TestAlertAndRecover(t *testing.T) {
	// 测试耗时告警
	testAlertAndRecover(200, 200, t)
	// 测试成功率告警
	testAlertAndRecover(200, 500, t)
}

// 同时测试告警和恢复
func testAlertAndRecover(ms uint32, status int, t *testing.T) {
	var alertTimes = 0
	var recoverTimes = 0
	cycle := 100
	var testReportClient = Register(ReportClientConfig {
		Name: "告警测试",
		StatisticalCycle: cycle,										// 上报统计周期100ms
		DefaultFastTime: ms,											// 默认服务的时间达标为200ms以内
		AlertCaller: func(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData) {
			// 告警只会触发1次 所以最终alertAlready应该为true  如果触发两次  则alertAlready为false  逻辑出错
			alertTimes++
		},
		RecoverCaller: func(clientName string, interfaceName string, alertType AlertType, recentOutputData []OutPutData) {
			recoverTimes++
		},
	})
	timeout := make(chan bool)
	defer close(timeout)
	time.AfterFunc(time.Duration(14 * cycle) * time.Millisecond, func() {
		if alertTimes != 1 || recoverTimes != 1 {
			t.Error("告警成功：", alertTimes, "恢复通知成功：", recoverTimes)
		}
		timeout <- true
	})
	// 3次触达告警  7次应该满足告警条件  但只触达一次
	for i := 0; i < 7; i++ {
		// 每个时间单位内随机上报10到59次
		times := rand.New(rand.NewSource(time.Now().Unix())).Int() % 50 + 10
		for j := 0; j < times; j++ {
			testReportClient.Report("GET - 测试接口", ms + 1, status)
		}
		time.Sleep(time.Duration(cycle) * time.Millisecond)
	}
	// 3次触达恢复通知  7次应该满足恢复条件  但只触达一次
	for i := 0; i < 7; i++ {
		// 每个时间单位内随机上报10到59次
		times := rand.New(rand.NewSource(time.Now().Unix())).Int() % 50 + 10
		for j := 0; j < times; j++ {
			testReportClient.Report("GET - 测试接口", ms - 1, 200)
		}
		time.Sleep(time.Duration(cycle) * time.Millisecond)
	}
	<- timeout
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