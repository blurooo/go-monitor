package monitor

import (
	"time"
	"strconv"
	"strings"
)

// 一个时间周期内最终统计输出的数据
type OutPutData struct {
	// 数据生成时间
	Timestamp time.Time `json:"timestamp"`
	// 客户端命名
	ClientName string `json:"clientName"`
	// 接口命名
	InterfaceName string `json:"interfaceName"`
	// 调用总次数
	Count uint32 `json:"count"`
	// 成功总数
	SuccessCount uint32 `json:"successCount"`
	// 成功率
	SuccessRate float64	`json:"successRate"`
	// 成功平均耗时
	SuccessMsAver uint32 `json:"successMsAver"`
	// 成功最大耗时
	MaxMs uint32 `json:"maxMs"`
	// 成功最小耗时
	MinMs uint32 `json:"minMs"`
	// 时间达标总数
	FastCount uint32 `json:"fastCount"`
	// 时间达标率
	FastRate float64 `json:"fastRate"`
	// 失败总数
	FailCount uint32 `json:"failCount"`
	// 失败分布 按照状态码分
	FailDistribution map[string]uint32 `json:"failDistribution"`
	// 时延分布情况
	TimeConsumingDistribution map[string]uint32 `json:"timeConsumingDistribution"`
}

// 存储一些最近状态，以用于实现告警、恢复等机制
type alertStatus struct {
	recentAlertOutput   []OutPutData // 最近连续几次失败的数据
	recentRecoverOutput []OutPutData // 自最近一次告警之后，连续成功的几次数据
	curState            AlertType    // 当前是否处于告警之后检测恢复的状态
}

// 周期性启动分析任务
func (c *ReportClientConfig) scheduleTask() {
	// 定时统计
	t := time.NewTicker(time.Duration(c.StatisticalCycle) * time.Millisecond)
	for curTime := range t.C {
		c.taskChannel <- &taskQueue {
			taskType: CLEAR,
			data: clearData {
				Time: curTime,
			},
		}
	}
}

// 分析统计
func (c *ReportClientConfig) statistics() {
	// 以具体条目为单位进行统计分析
	for collectedData := range c.statisticsChannel {
		// 常规指标统计
		outputData := OutPutData {}
		outputData.ClientName = c.Name
		outputData.InterfaceName = collectedData.Name
		outputData.Count = collectedData.FailCount + collectedData.SuccessCount
		outputData.SuccessRate = float64(collectedData.SuccessCount) / float64(outputData.Count)
		outputData.FastRate = float64(collectedData.FastCount) / float64(outputData.Count)
		outputData.FastCount = collectedData.FastCount
		outputData.SuccessMsAver = uint32(float64(collectedData.SuccessMsCount) / float64(outputData.Count))
		outputData.SuccessCount = collectedData.SuccessCount
		outputData.FailCount = collectedData.FailCount
		outputData.MaxMs = collectedData.MaxMs
		outputData.MinMs = collectedData.MinMs
		outputData.Timestamp = collectedData.Time.UTC()
		outputData.TimeConsumingDistribution = map[string]uint32 {}
		outputData.FailDistribution = map[string]uint32 {}


		// 时延分布统计
		scope := (collectedData.Config.TimeConsumingDistributionMax - collectedData.Config.TimeConsumingDistributionMin) / uint32(collectedData.Config.TimeConsumingDistributionSplit - 2)
		// 计算第一个区间
		outputData.TimeConsumingDistribution["<" + strconv.FormatUint(uint64(collectedData.Config.TimeConsumingDistributionMin), 10)] = collectedData.TimeConsumingDistribution[0]
		// 计算最后一个区间
		outputData.TimeConsumingDistribution[">" + strconv.FormatUint(uint64(collectedData.Config.TimeConsumingDistributionMax), 10)] = collectedData.TimeConsumingDistribution[collectedData.Config.TimeConsumingDistributionSplit - 1]
		// 计算剩余区间
		for i := 1; i < collectedData.Config.TimeConsumingDistributionSplit - 1; i++ {
			start := int(collectedData.Config.TimeConsumingDistributionMin + uint32(i - 1) * scope)
			var end int
			if i == collectedData.Config.TimeConsumingDistributionSplit - 1 {
				end = int(collectedData.Config.TimeConsumingDistributionMax)
			} else {
				end = int(collectedData.Config.TimeConsumingDistributionMin + uint32(i) * scope)
			}
			outputData.TimeConsumingDistribution[strconv.Itoa(start) + "~" + strconv.Itoa(end)] = collectedData.TimeConsumingDistribution[i]
		}


		// 失败分布统计
		for status, count := range collectedData.FailDistribution {
			var name string
			if c.GetCodeFeature != nil {
				_, name = c.GetCodeFeature(status)
			} else if s, ok := c.CodeFeatureMap[status]; ok && s.Name != "" {
				name = s.Name
			}
			if name != "" {
				outputData.FailDistribution[name] = count
			} else {
				outputData.FailDistribution[strings.Replace(c.DefaultFailDistributionFormat, "%code", strconv.Itoa(status), 1)] = count
			}
		}

		// 告警分析：由于告警分析存在对定制化告警函数的调用可能性，无法预估性能，所以启用新的gorouting去执行避免不可预测的风险
		//c.alertAnalyze(collectedData.Name, outputData)

		// 输出最终统计数据
		if c.OutputCaller != nil {
			// 同理，但凡外部自定义函数的调用应当启用新的gorouting去执行
			go c.OutputCaller(&outputData)
		}
		defaultOutputCaller(&outputData)
	}
}

// 告警相关的分析
func (c *ReportClientConfig) alertAnalyze(entryName string, outputData OutPutData) {
	// 时延达标率告警和恢复分析
	if _, ok := c.recentFastRateStatus[entryName]; !ok {
		c.recentFastRateStatus[entryName] = &alertStatus {
			recentAlertOutput: make([]OutPutData, 0),
		}
	}
	curFastRateStatus := c.recentFastRateStatus[entryName]
	if _, ok := c.recentSuccessRateStatus[entryName]; !ok {
		c.recentSuccessRateStatus[entryName] = &alertStatus {
			recentAlertOutput: make([]OutPutData, 0),
		}
	}
	curSuccessRateStatus := c.recentSuccessRateStatus[entryName]
	// 时延不达标告警只在有成功请求时才触发统计
	if outputData.SuccessCount > 0 && outputData.FastRate < c.FastRate {
		// 每次失败都将重置恢复计数
		if len(curFastRateStatus.recentRecoverOutput) > 0 {
			curFastRateStatus.recentRecoverOutput = curFastRateStatus.recentRecoverOutput[:0]
		}
		curFastRateStatus.recentAlertOutput = append(curFastRateStatus.recentAlertOutput, outputData)
		if curFastRateStatus.curState == NONE && len(curFastRateStatus.recentAlertOutput) >= c.AlertForBadFastRateReachedTimes {
			// 标记出当前告警的状态
			curFastRateStatus.curState = SLOW
			// 触发连续耗时不达标告警
			if c.AlertCaller != nil {
				c.AlertCaller(c.Name, entryName, SLOW, curFastRateStatus.recentAlertOutput)
			} else {
				defaultAlert(c.Name, entryName, SLOW, curFastRateStatus.recentAlertOutput)
			}
			curFastRateStatus.recentAlertOutput = curFastRateStatus.recentAlertOutput[:0]
		}
	} else {
		// 只要一次成功就清空原有不健康记录，实测比判断长度是否大于0再去清空性能要略优
		curFastRateStatus.recentAlertOutput = curFastRateStatus.recentAlertOutput[:0]
		// 处于告警状态时 每次成功都累计恢复次数
		if curFastRateStatus.curState == SLOW {
			curFastRateStatus.recentRecoverOutput = append(curFastRateStatus.recentRecoverOutput, outputData)
			if len(curFastRateStatus.recentRecoverOutput) >= c.AlertForGreatFastRateReachedTimes {
				// 触发恢复通知
				if c.RecoverCaller != nil {
					c.RecoverCaller(c.Name, entryName, SLOW, curFastRateStatus.recentRecoverOutput)
				} else {
					defaultRecover(c.Name, entryName, SLOW, curFastRateStatus.recentRecoverOutput)
				}
				// 重置标志
				curFastRateStatus.curState = NONE
				curFastRateStatus.recentAlertOutput = curFastRateStatus.recentAlertOutput[:0]
			}
		}
	}

	// 访问成功率告警与恢复分析
	if outputData.SuccessRate < c.SuccessRate {
		// 每次失败都将重置恢复计数
		if len(curSuccessRateStatus.recentRecoverOutput) > 0 {
			curSuccessRateStatus.recentRecoverOutput = curSuccessRateStatus.recentRecoverOutput[:0]
		}
		curSuccessRateStatus.recentAlertOutput = append(curSuccessRateStatus.recentAlertOutput, outputData)
		if curSuccessRateStatus.curState == NONE && len(curSuccessRateStatus.recentAlertOutput) >= c.AlertForBadSuccessRateReachedTimes {
			// 标记出当前告警的状态
			curSuccessRateStatus.curState = FAIL
			// 触发连续耗时不达标告警
			if c.AlertCaller != nil {
				c.AlertCaller(c.Name, entryName, FAIL, curSuccessRateStatus.recentAlertOutput)
			} else {
				defaultAlert(c.Name, entryName, FAIL, curSuccessRateStatus.recentAlertOutput)
			}
			curSuccessRateStatus.recentAlertOutput = curSuccessRateStatus.recentAlertOutput[:0]
		}
	} else {
		// 只要一次成功就清空原有不健康记录
		curSuccessRateStatus.recentAlertOutput = curSuccessRateStatus.recentAlertOutput[:0]
		// 处于告警状态时 每次成功都累计恢复次数
		if curSuccessRateStatus.curState == FAIL {
			curSuccessRateStatus.recentRecoverOutput = append(curSuccessRateStatus.recentRecoverOutput, outputData)
			if len(curSuccessRateStatus.recentRecoverOutput) >= c.AlertForGreatSuccessRateReachedTimes {
				// 触发恢复通知
				if c.RecoverCaller != nil {
					c.RecoverCaller(c.Name, entryName, FAIL, curSuccessRateStatus.recentRecoverOutput)
				} else {
					defaultRecover(c.Name, entryName, FAIL, curSuccessRateStatus.recentRecoverOutput)
				}
				// 重置标志
				curSuccessRateStatus.curState = NONE
				curSuccessRateStatus.recentAlertOutput = curSuccessRateStatus.recentAlertOutput[:0]
			}
		}
	}
}