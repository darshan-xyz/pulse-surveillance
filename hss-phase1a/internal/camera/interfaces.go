package camera

// Camera interface standardizes interaction with RTSP, ONVIF, or test cameras.
type Camera interface {
	ID() string
	Name() string
	Status() string
	Capabilities() []string
	MainStreamURL() string
	SubStreamURL() string
	Connect() error
	Disconnect() error
}
