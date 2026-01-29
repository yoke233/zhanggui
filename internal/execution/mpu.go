package execution

type MPU struct {
	MPUID  string
	TeamID string
	Role   string
	Kind   string // report_section|ppt_slide|quality
	Title  string
}
