package camera

import (
	"hss/internal/media"
)

type SyntheticCamera struct {
	id     string
	name   string
	status string
}

func NewSyntheticCamera(id, name string) *SyntheticCamera {
	if id == "" {
		id = "test-camera-1"
	}
	if name == "" {
		name = "Synthetic Test Pattern Camera"
	}
	return &SyntheticCamera{
		id:     id,
		name:   name,
		status: "online",
	}
}

func (c *SyntheticCamera) ID() string {
	return c.id
}

func (c *SyntheticCamera) Name() string {
	return c.name
}

func (c *SyntheticCamera) Status() string {
	return c.status
}

func (c *SyntheticCamera) Capabilities() []string {
	return []string{"synthetic", "h264", "test-pattern"}
}

func (c *SyntheticCamera) MainStreamURL() string {
	return "synthetic://test-pattern"
}

func (c *SyntheticCamera) SubStreamURL() string {
	return "synthetic://test-pattern"
}

func (c *SyntheticCamera) Connect() error {
	return media.DefaultEngine.AttachCamera(c)
}

func (c *SyntheticCamera) Disconnect() error {
	return media.DefaultEngine.DetachCamera(c.id)
}
