package workitem

type Stage string

const (
	StageDraft Stage = "draft"
	StageFile  Stage = "file"
	StageImpl  Stage = "impl"
	StageShip  Stage = "ship"
)

// StagesFor returns the pipeline stages a WorkItem.Type needs.
// Plan §"Triage decides which stages run per WorkItem".
func StagesFor(t Type) []Stage {
	switch t {
	case TypeCapabilityGap:
		return []Stage{StageDraft, StageFile, StageImpl, StageShip}
	case TypeFeatureRequest, TypeBugReport, TypeManifestFeature:
		return []Stage{StageImpl, StageShip}
	case TypeInlineTask:
		return []Stage{StageImpl, StageShip}
	default:
		return nil
	}
}
