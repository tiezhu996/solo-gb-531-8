package constants

// ReviewOrigin 记录复评记录是由到期/失效流程触发，还是人工登记的复评任务。
type ReviewOrigin string

const (
	ReviewOriginManual  ReviewOrigin = "manual"
	ReviewOriginExpiry  ReviewOrigin = "expiry"
	ReviewOriginFailure ReviewOrigin = "failure"
)

func ReviewOriginValues() []string {
	return []string{string(ReviewOriginManual), string(ReviewOriginExpiry), string(ReviewOriginFailure)}
}
