package audit

type EventType string
type Action string
type TargetType string
type ActorType string

// These constants must match the enum definitions in configs/argus/config.yaml.
// A unit test (enums_test.go) asserts they are defined in that configuration.
const (
	EventConsignment EventType = "CONSIGNMENT_EVENT"
	EventTask        EventType = "TASK_EVENT"
	EventStorage     EventType = "STORAGE_EVENT"
	EventPayment     EventType = "PAYMENT_EVENT"
	EventUserMgmt    EventType = "USER_MANAGEMENT"

	ActionCreate Action = "CREATE"
	ActionRead   Action = "READ"
	ActionUpdate Action = "UPDATE"
	ActionDelete Action = "DELETE"

	TargetConsignment TargetType = "CONSIGNMENT"
	TargetTask        TargetType = "TASK"
	TargetStorage     TargetType = "STORAGE_OBJECT"
	TargetPayment     TargetType = "PAYMENT"

	ActorAdmin   ActorType = "ADMIN"
	ActorMember  ActorType = "MEMBER"
	ActorService ActorType = "SERVICE"
	ActorSystem  ActorType = "SYSTEM"
)
