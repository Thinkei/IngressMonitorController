package controllers

import (
	"github.com/stakater/IngressMonitorController/pkg/models"
	"github.com/stakater/IngressMonitorController/pkg/monitors"
)

func findMonitorByName(monitorService monitors.MonitorServiceProxy, monitorName string) (*models.Monitor, error) {

	monitor, err := monitorService.GetByName(monitorName)
	// Monitor Exists
	if monitor != nil && err == nil {
		return monitor, nil
	}
	return nil, err
}
