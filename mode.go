package eventcatcher

type Mode string

const (
	Sandbox    Mode = "Sandbox"
	Production Mode = "Production"
)

func GetBaseURL(mode Mode) string {
	var baseURLs = map[Mode]string{
		Sandbox:    "https://event-gateway-dev.layerg.xyz",
		Production: "https://event-gateway-dev.layerg.xyz",
	}

	return baseURLs[mode]
}
