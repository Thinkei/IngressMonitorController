package pingdom

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/jsdidierlaurent/go-pingdom/pingdom"
	"github.com/stakater/IngressMonitorController/pkg/config"
	"github.com/stakater/IngressMonitorController/pkg/models"
	"github.com/stakater/IngressMonitorController/pkg/util"
)

const (
	PingdomResolutionAnnotation               = "pingdom.monitor.stakater.com/resolution"
	PingdomSendNotificationWhenDownAnnotation = "pingdom.monitor.stakater.com/send-notification-when-down"
	PingdomPausedAnnotation                   = "pingdom.monitor.stakater.com/paused"
	PingdomNotifyWhenBackUpAnnotation         = "pingdom.monitor.stakater.com/notify-when-back-up"
	PingdomRequestHeadersAnnotation           = "pingdom.monitor.stakater.com/request-headers"
	PingdomBasicAuthUser                      = "pingdom.monitor.stakater.com/basic-auth-user"
	PingdomShouldContainString                = "pingdom.monitor.stakater.com/should-contain"
	PingdomTags                               = "pingdom.monitor.stakater.com/tags"
	PingdomAlertIntegrations                  = "pingdom.monitor.stakater.com/alert-integrations"
	PingdomAlertContacts                      = "pingdom.monitor.stakater.com/alert-contacts"
)

// PingdomMonitorService interfaces with MonitorService
type PingdomMonitorService struct {
	apiKey            string
	alertContacts     string
	alertIntegrations string
	client            *pingdom.Client
}

func (service *PingdomMonitorService) Setup(p config.Provider) {
	service.apiKey = p.ApiKey
	service.alertContacts = p.AlertContacts
	service.alertIntegrations = p.AlertIntegrations

	// log.Info("apiKey: " + service.apiKey)

	client, err := pingdom.NewClientWithConfig(pingdom.ClientConfig{
		APIToken: service.apiKey,
	})

	if err != nil {
		log.Error("Init client error: ", err.Error())
	} else {
		service.client = client
	}
}

func (service *PingdomMonitorService) GetByName(name string) (*models.Monitor, error) {
	var match *models.Monitor

	monitors := service.GetAll()
	for _, mon := range monitors {
		if mon.Name == name {
			return &mon, nil
		}
	}

	return match, fmt.Errorf("Unable to locate monitor with name %v", name)
}

func (service *PingdomMonitorService) GetAll() []models.Monitor {
	var monitors []models.Monitor

	checks, err := service.client.Checks.List()
	if err != nil {
		log.Error("Error received while listing checks: ", err.Error())
		return nil
	}
	for _, mon := range checks {
		newMon := models.Monitor{
			URL:  mon.Hostname,
			ID:   fmt.Sprintf("%v", mon.ID),
			Name: mon.Name,
		}
		monitors = append(monitors, newMon)
	}

	return monitors
}

func (service *PingdomMonitorService) Add(m models.Monitor) {
	httpCheck := service.createHttpCheck(m)

	_, err := service.client.Checks.Create(&httpCheck)
	if err != nil {
		log.Error("Error Adding Monitor: ", err.Error())
	} else {
		log.Println("Added monitor for: ", m.Name)
	}
}

func (service *PingdomMonitorService) Update(m models.Monitor) {
	httpCheck := service.createHttpCheck(m)
	monitorID, _ := strconv.Atoi(m.ID)

	resp, err := service.client.Checks.Update(monitorID, &httpCheck)
	if err != nil {
		log.Error("Error updating Monitor: ", err.Error())
	} else {
		log.Println("Updated Monitor: ", resp)
	}
}

func (service *PingdomMonitorService) Remove(m models.Monitor) {
	monitorID, _ := strconv.Atoi(m.ID)

	resp, err := service.client.Checks.Delete(monitorID)
	if err != nil {
		log.Error("Error deleting Monitor: ", err.Error())
	} else {
		log.Println("Delete Monitor: ", resp)
	}
}

func (service *PingdomMonitorService) createHttpCheck(monitor models.Monitor) pingdom.HttpCheck {
	httpCheck := pingdom.HttpCheck{}
	url, err := url.Parse(monitor.URL)
	if err != nil {
		log.Error("Unable to parse the URL: ", monitor.URL)
	}

	if url.Scheme == "https" {
		httpCheck.Encryption = true
	} else {
		httpCheck.Encryption = false
	}

	httpCheck.Hostname = url.Host
	httpCheck.Url = url.Path
	httpCheck.Name = monitor.Name

	userIdsStringArray := strings.Split(service.alertContacts, "-")

	if userIds, err := util.SliceAtoi(userIdsStringArray); err != nil {
		log.Println(err.Error())
	} else {
		httpCheck.UserIds = userIds
	}

	integrationIdsStringArray := strings.Split(service.alertIntegrations, "-")

	if integrationIds, err := util.SliceAtoi(integrationIdsStringArray); err != nil {
		log.Println(err.Error())
	} else {
		httpCheck.IntegrationIds = integrationIds
	}

	service.addAnnotationConfigToHttpCheck(&httpCheck, monitor.Annotations)

	return httpCheck
}

func (service *PingdomMonitorService) addAnnotationConfigToHttpCheck(httpCheck *pingdom.HttpCheck, annotations map[string]string) {
	// Read known annotations, try to map them to pingdom configs
	// set some default values if we can't find them

	if value, ok := annotations[PingdomAlertContacts]; ok {
		userIdsStringArray := strings.Split(value, "-")

		if userIds, err := util.SliceAtoi(userIdsStringArray); err != nil {
			log.Error("Error decoding user ids annotation", err.Error())
		} else {
			httpCheck.UserIds = userIds
		}
	}

	if value, ok := annotations[PingdomAlertIntegrations]; ok {
		integrationIdsStringArray := strings.Split(value, "-")

		if integrationIds, err := util.SliceAtoi(integrationIdsStringArray); err != nil {
			log.Error("Error decoding integration ids annotation into integers", err.Error())
		} else {
			httpCheck.IntegrationIds = integrationIds
		}
	}

	if value, ok := annotations[PingdomNotifyWhenBackUpAnnotation]; ok {
		boolValue, err := strconv.ParseBool(value)
		if err == nil {
			httpCheck.NotifyWhenBackup = boolValue
		}
	}

	if value, ok := annotations[PingdomPausedAnnotation]; ok {
		boolValue, err := strconv.ParseBool(value)
		if err == nil {
			httpCheck.Paused = boolValue
		}
	}

	if value, ok := annotations[PingdomResolutionAnnotation]; ok {
		intValue, err := strconv.Atoi(value)
		if err == nil {
			httpCheck.Resolution = intValue
		} else {
			log.Error("Error decoding input into an integer")
			httpCheck.Resolution = 1
		}
	} else {
		httpCheck.Resolution = 1
	}

	if value, ok := annotations[PingdomSendNotificationWhenDownAnnotation]; ok {
		intValue, err := strconv.Atoi(value)
		if err == nil {
			httpCheck.SendNotificationWhenDown = intValue
		} else {
			log.Error("Error decoding input into an integer")
			httpCheck.SendNotificationWhenDown = 3
		}
	} else {
		httpCheck.SendNotificationWhenDown = 3
	}

	if value, ok := annotations[PingdomRequestHeadersAnnotation]; ok {

		httpCheck.RequestHeaders = make(map[string]string)
		err := json.Unmarshal([]byte(value), &httpCheck.RequestHeaders)
		if err != nil {
			log.Error("Error Converting from string to JSON object")
		}
	}

	// Does an annotation want to use basic auth
	if userValue, ok := annotations[PingdomBasicAuthUser]; ok {
		// Annotation should be set to the username to set on the httpCheck
		// Environment variable should define the password
		// Mounted via a secret; key is the username, value the password
		passwordValue := os.Getenv(userValue)
		if passwordValue != "" {
			// Env variable set, pass user/pass to httpCheck
			httpCheck.Username = userValue
			httpCheck.Password = passwordValue
			log.Println("Basic auth requirement detected. Setting username and password for httpCheck")
		} else {
			log.Error("Error reading basic auth password from environment variable")
		}
	}

	// Does an annotation want to set a "should contain" string
	if containValue, ok := annotations[PingdomShouldContainString]; ok {
		if containValue != "" {
			httpCheck.ShouldContain = containValue
			log.Println("Should contain annotation detected. Setting Should Contain string: ", containValue)
		}
	}

	// Does an annotation want to set any "tags"
	// Tags should be a single word or multiple comma-seperated words
	if tagValue, ok := annotations[PingdomTags]; ok {
		if tagValue != "" && !strings.Contains(tagValue, " ") {
			httpCheck.Tags = tagValue
			log.Println("Tags annotation detected. Setting Tags as: ", tagValue)
		} else {
			log.Println("Tag string should not contain spaces. Not applying tags.")
		}
	}
}
