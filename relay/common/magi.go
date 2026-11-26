package common

const (
	HiggsfieldMagiProfileID = "sandai-higgsfield-magi-2"
	ModelSellMagiProfileID  = "sandai-modelsell-magi-2"
	OfficialMagiProfileID   = "sandai-official-magi-2"
	SandGenerationsPath     = "/v2/generations"
	SandArkTasksPath        = "/compatible/ark/api/v3/contents/generations/tasks"
	MagiNativeModeKey       = "magi_native_mode"
	MagiModel               = "magi-2"
)

func IsMagiVideoProfile(id string) bool {
	return id == HiggsfieldMagiProfileID || id == ModelSellMagiProfileID || id == OfficialMagiProfileID
}
