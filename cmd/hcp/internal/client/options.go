package client

import (
	"github.com/sirupsen/logrus"
)

type WithVeleroNamespace string

func (w WithVeleroNamespace) ConfigureManagementClusterClient(c *ManagementClusterClientConfig) {
	c.VeleroNamespace = string(w)
}

type WithLogger struct {
	Logger *logrus.Logger
}

func (w WithLogger) ConfigureManagementClusterClient(c *ManagementClusterClientConfig) {
	c.Logger = w.Logger
}

func (w WithLogger) ConfigureOCMClient(c *OCMClientConfig) {
	c.Logger = w.Logger
}
