package daemon

const defaultServiceName = "ZeitBoardServer"

func Run(configPath, serviceName string) error {
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	return runPlatform(configPath, serviceName)
}
