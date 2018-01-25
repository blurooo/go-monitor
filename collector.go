package monitor

import "time"

// 每一条上报数据都会流入收集模块，收集模块只做一些简单的数据记录
type reportData struct {
	// 条目的唯一命名
	Name string
	// 成功总耗时
	SuccessMsCount uint64
	// 成功最大耗时
	MaxMs uint32
	// 成功最小耗时
	MinMs uint32
	// 成功总数
	SuccessCount uint32
	// 时间达标总数
	FastCount uint32
	// 失败总数
	FailCount uint32
	// 失败分布 按照状态码分
	FailDistribution map[int]uint32
	// 时延分布情况
	TimeConsumingDistribution []uint32
	// 条目的配置
	Config *EntryConfig
	// 本次统计的时间
	Time time.Time
}

// 条目统计相关的更详尽配置
type EntryConfig struct {
	// 时间达标的最大耗时，默认为500ms
	FastLessThan uint32
	// 耗时分布区间个数，默认为10，至少为3，最多为20
	// 第一个区间用于标注小于TimeConsumingMin的个数
	// 最后一个区间用于标注大于TimeConsumingMax的个数
	// 剩余的(TimeConsumingDistributionSplit - 2)个区间，范围为(最大耗时-最小耗时)/(区间数-2)
	TimeConsumingDistributionSplit int
	// 耗时分布区间计最大耗时，默认为500ms
	TimeConsumingDistributionMax uint32
	// 耗时分布区间计最小耗时，默认为50ms，至少为1
	TimeConsumingDistributionMin uint32
	// 计算出区间
	timeConsumingRange uint32
}

// 定义了一些默认的条目统计相关的属性
var defaultEntryConfig = &EntryConfig {
	FastLessThan:					500,
	TimeConsumingDistributionSplit: 10,
	TimeConsumingDistributionMax:	500,
	TimeConsumingDistributionMin:	100,
	timeConsumingRange:				(500 - 100) / (10 - 2),
}

// 统一通过此方法获取条目的配置，数据变得规范
func (c *ReportClientConfig) getEntryConfig(name string) *EntryConfig {
	if curEntryConfig, ok := c.entryConfigMap[name]; ok {
		return &curEntryConfig
	}
	return defaultEntryConfig
}

// 添加条目的自定义属性
func (c *ReportClientConfig) AddEntryConfig(name string, entryConfig EntryConfig) {
	if entryConfig.FastLessThan <= 0 {
		entryConfig.FastLessThan = 500
	}
	// 考虑到实际分布意义，最小应有三个区间，最大只能有20个区间
	if entryConfig.TimeConsumingDistributionSplit < 3 || entryConfig.TimeConsumingDistributionSplit > 20 {
		entryConfig.TimeConsumingDistributionSplit = 10
	}
	if entryConfig.TimeConsumingDistributionMax <= 0 {
		entryConfig.TimeConsumingDistributionMax = 500
	}
	if entryConfig.TimeConsumingDistributionMin <= 0 {
		entryConfig.TimeConsumingDistributionMin = 50
	}
	if entryConfig.TimeConsumingDistributionMax <= entryConfig.TimeConsumingDistributionMin {
		panic("耗时最长值必须大于耗时最短值")
	}
	entryConfig.timeConsumingRange = (entryConfig.TimeConsumingDistributionMax - entryConfig.TimeConsumingDistributionMin) / uint32(entryConfig.TimeConsumingDistributionSplit - 2)
	c.entryConfigMap[name] = entryConfig
}

// 收集
func (c *ReportClientConfig) collect() {
	// 监听本客户端的上报信道
	for t := range c.taskChannel {
		// 服务端上报类型的统计任务
		if t.taskType == SERVER {
			curReportServerData := t.data.(reportServer)
			c.serverTask(&curReportServerData)

		} else if t.taskType == CLEAR {		// 清理旧统计数据的任务
			curClearData := t.data.(clearData)
			c.clearTask(&curClearData)
		}
	}
}

// 清理任务
func (c *ReportClientConfig) clearTask(curClearData *clearData) {
	curCollectData := c.collectDataMap[curClearData.Name]
	// 只在有上报记录时才做清理
	if curCollectData.SuccessCount != 0 || curCollectData.FailCount != 0 {
		collectedData := *curCollectData
		collectedData.Time = curClearData.Time
		// 拷贝一份数据流入分析
		c.statisticsChannel <- collectedData
		// 清空旧数据
		curCollectData.MinMs = 0
		curCollectData.MaxMs = 0
		curCollectData.FailCount = 0
		curCollectData.SuccessCount = 0
		curCollectData.SuccessMsCount = 0
		curCollectData.FastCount = 0
		curCollectData.FailDistribution = map[int]uint32 {}
		curCollectData.TimeConsumingDistribution = make([]uint32, curCollectData.Config.TimeConsumingDistributionSplit)
	}
}

// 服务端上报类型的收集任务
func (c *ReportClientConfig) serverTask(curReportServerData *reportServer) {
	// 如果该条目的收集数据不存在则初始化它
	if c.collectDataMap[curReportServerData.Name] == nil {
		c.collectDataMap[curReportServerData.Name] = &reportData {
			Name: curReportServerData.Name,
			Config: c.getEntryConfig(curReportServerData.Name),
			FailDistribution: map[int]uint32 {},
		}
	}
	curCollectData := c.collectDataMap[curReportServerData.Name]
	if curCollectData.TimeConsumingDistribution == nil {
		// 先分配空间
		curCollectData.TimeConsumingDistribution = make([]uint32, curCollectData.Config.TimeConsumingDistributionSplit)
	}
	var success bool
	if c.GetCodeFeature != nil {
		success, _ = c.GetCodeFeature(curReportServerData.Code)
	} else if s, ok := c.CodeFeatureMap[curReportServerData.Code]; ok {
		success = s.Success
	}
	// 命中成功状态码
	if success {
		curCollectData.SuccessCount++
		if curCollectData.MinMs == 0 {
			curCollectData.MinMs = curReportServerData.Ms
		} else if curReportServerData.Ms < curCollectData.MinMs {
			curCollectData.MinMs = curReportServerData.Ms
		}
		if curCollectData.MaxMs == 0 {
			curCollectData.MaxMs = curReportServerData.Ms
		} else if curReportServerData.Ms > curCollectData.MaxMs {
			curCollectData.MaxMs = curReportServerData.Ms
		}
		curCollectData.SuccessMsCount += uint64(curReportServerData.Ms)
		// 耗时小于区间最小  归类为第一区间
		if curReportServerData.Ms < curCollectData.Config.TimeConsumingDistributionMin {
			curCollectData.TimeConsumingDistribution[0] += 1
		} else if curReportServerData.Ms >= curCollectData.Config.TimeConsumingDistributionMax {
			// 耗时大于等于区间最大  归类为最后一个区间
			curCollectData.TimeConsumingDistribution[curCollectData.Config.TimeConsumingDistributionSplit - 1] += 1
		} else {
			// 其他情况落在对应的耗时区间
			curCollectData.TimeConsumingDistribution[(curReportServerData.Ms - curCollectData.Config.TimeConsumingDistributionMin) / curCollectData.Config.timeConsumingRange + 1] += 1
		}
		if curReportServerData.Ms <= curCollectData.Config.FastLessThan {
			curCollectData.FastCount++
		}
	} else {
		curCollectData.FailCount++
		curCollectData.FailDistribution[curReportServerData.Code]++
	}
}