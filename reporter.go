package monitor

import "time"

// 服务质量统计任务携带的数据（目前暂不考虑上报时间）
type reportServer struct {
	// 命名，应当确保唯一。
	// 用在接口上报时可以设置为访问地址和请求方法的组合。
	// 部分接口可能会携带路径参数或请求参数，如果不做处理，监控结果将与预期不符，建议提前将请求参数去掉、将路径参数格式化
	Name string
	// 耗时，全部时间单位都以毫秒计
	Ms uint32
	// 状态码，用在接口上报时可以设置为请求状态码，也可以自定义一套映射规则
	Code int
}

// 清理任务携带的数据
type clearData struct {
	Name string
	Time time.Time
}

// 任务队列，将共享变量的读写汇总为任务队列，避免锁的使用
type taskQueue struct {
	taskType TaskType
	data interface {}
}

// 上报的名称 可以为接口命名，也可以直接传入url
// 通常以url或name作为报告归类标准，但如果上报的是url，在restful风格的接口规范里并不能直接归类，此时请求方法就变得尤为重要
// 使用channel作为上报方式，而不考虑直接同步上报分析是为了将上报的成本尽可能降低，只做该做的事情，并即刻返回
// 因为在并发的场景下，分析需要申请并发锁，并发锁则存在阻塞（锁被占用），所以分析过程再轻量也可能会导致上报影响到主流程的进展
// report的数据可以来源于任何地方，包括接口上报，服务内嵌等
func (c *ReportClientConfig) Report(name string, ms uint32, code int) {
	if c.taskChannel == nil {
		panic("请首先注册该上报类型")
	}
	c.taskChannel <- &taskQueue {
		taskType: SERVER,
		data: reportServer {
			Code: 	code,
			Ms: 	ms,
			Name:   name,
		},
	}
}