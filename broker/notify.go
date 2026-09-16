package broker

import "context"

// Notification 描述一条待批准的请求。
//
// FCM 只需要 RequestID（推送文案不写条目名和路径，详情进 App 再看）。
// ApproveSig / DenySig 留给「能把决策一起带出去」的推送通道；Broker 负责生成，
// 真正的校验在 claimDecision（签名或设备令牌，二者取一）。
type Notification struct {
	RequestID  string
	ApproveSig string
	DenySig    string
}

// Notifier 是可插拔的推送通道。生产用的是 FCMClient（见 server.push）；
// 这个接口留作测试替身和以后接别的通道。
type Notifier interface {
	Notify(context.Context, Notification) error
}
