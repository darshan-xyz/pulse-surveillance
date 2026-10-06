package camera

import (
	"hss/internal/media"
)

type GenericCamera struct {
	id         string
	name       string
	mainStream string
}

func NewGenericCamera(id, name, mainStream string) *GenericCamera {
	return &GenericCamera{
		id:         id,
		name:       name,
		mainStream: mainStream,
	}
}

func (c *GenericCamera) ID() string {
	return c.id
}

func (c *GenericCamera) Name() string {
	return c.name
}

func (c *GenericCamera) Status() string {
	return "unknown"
}

func (c *GenericCamera) Capabilities() []string {
	return []string{"rtsp", "h264"}
}

func (c *GenericCamera) MainStreamURL() string {
	return c.mainStream
}

func (c *GenericCamera) SubStreamURL() string {
	return c.mainStream // fallback to main
}

func (c *GenericCamera) Connect() error {
	return media.DefaultEngine.AttachCamera(c)
}

func (c *GenericCamera) Disconnect() error {
	return media.DefaultEngine.DetachCamera(c.id)
}
