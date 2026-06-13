package audit

// These constants MUST match configs/argus/enums.yaml.
// A unit test (enums_test.go) asserts they are a subset of that file.
const (
	EventConsignment = "CONSIGNMENT_EVENT"
	EventTask        = "TASK_EVENT"
	EventStorage     = "STORAGE_EVENT"
	EventPayment     = "PAYMENT_EVENT"
	EventUserMgmt    = "USER_MANAGEMENT"

	ActionCreate = "CREATE"
	ActionRead   = "READ"
	ActionUpdate = "UPDATE"
	ActionDelete = "DELETE"

	TargetConsignment = "CONSIGNMENT"
	TargetTask        = "TASK"
	TargetStorage     = "STORAGE_OBJECT"
	TargetPayment     = "PAYMENT"

	ActorAdmin   = "ADMIN"
	ActorMember  = "MEMBER"
	ActorService = "SERVICE"
	ActorSystem  = "SYSTEM"
)
