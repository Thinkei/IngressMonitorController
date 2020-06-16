package wrappers

import (
	"net/url"
	"path"

	log "github.com/sirupsen/logrus"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stakater/IngressMonitorController/pkg/constants"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

type RouteWrapper struct {
	Route      *routev1.Route
	Namespace  string
	KubeClient kubernetes.Interface
}

func (rw *RouteWrapper) supportsTLS() bool {
	if rw.Route.Spec.TLS != nil {
		return true
	}
	return false
}

func (rw *RouteWrapper) tryGetTLSHost() (string, bool) {
	if rw.supportsTLS() {
		return "https://" + rw.Route.Spec.Host, true
	}

	annotations := rw.Route.GetAnnotations()
	if value, ok := annotations[constants.ForceHTTPSAnnotation]; ok {
		if value == "true" {
			// Annotation exists and is enabled
			return "https://" + rw.Route.Spec.Host, true
		}
	}

	return "", false
}

func (rw *RouteWrapper) getHost() string {
	return "http://" + rw.Route.Spec.Host
}

func (rw *RouteWrapper) getRoutePort() string {
	if rw.Route.Spec.Port != nil && rw.Route.Spec.Port.TargetPort.String() != "" {
		return rw.Route.Spec.Port.TargetPort.String()
	}
	return ""
}

func (rw *RouteWrapper) getRouteSubPath() string {
	return rw.Route.Spec.Path
}

func (rw *RouteWrapper) hasService() (string, bool) {
	if rw.Route.Spec.To.Name != "" {
		return rw.Route.Spec.To.Name, true
	}
	return "", false
}

func (rw *RouteWrapper) tryGetHealthEndpointFromRoute() (string, bool) {

	serviceName, exists := rw.hasService()

	if !exists {
		return "", false
	}

	service, err := rw.KubeClient.Core().Services(rw.Route.Namespace).Get(serviceName, meta_v1.GetOptions{})
	if err != nil {
		log.WithFields(log.Fields{
			"service name": serviceName,
		}).Error("Get service from kubernetes cluster error: ", err.Error())
		return "", false
	}

	set := labels.Set(service.Spec.Selector)

	if pods, err := rw.KubeClient.Core().Pods(rw.Route.Namespace).List(meta_v1.ListOptions{LabelSelector: set.AsSelector().String()}); err != nil {
		log.WithFields(log.Fields{
			"service": service.GetName(),
		}).Error("Error to list pods: ", err.Error())
	} else if len(pods.Items) > 0 {
		pod := pods.Items[0]

		podContainers := pod.Spec.Containers

		if len(podContainers) == 1 {
			if podContainers[0].ReadinessProbe != nil && podContainers[0].ReadinessProbe.HTTPGet != nil {
				return podContainers[0].ReadinessProbe.HTTPGet.Path, true
			}
		} else {
			log.WithFields(log.Fields{
				"number of containers": len(podContainers),
				"service name":         serviceName,
			}).Info("Skipping health endpoint because number of containers > 1")
		}
	}

	return "", false
}

func (rw *RouteWrapper) GetURL() string {
	var URL string

	if host, exists := rw.tryGetTLSHost(); exists { // Get TLS Host if it exists
		URL = host
	} else {
		URL = rw.getHost() // Fallback for normal Host
	}

	// Convert url to url object
	u, err := url.Parse(URL)

	if err != nil {
		log.WithFields(log.Fields{
			"url": URL,
		}).Error("URL parsing error in getURL()", err.Error())
		return ""
	}

	annotations := rw.Route.GetAnnotations()

	if value, ok := annotations[constants.OverridePathAnnotation]; ok {
		u.Path = value
	} else {
		// Append subpath
		u.Path = path.Join(u.Path, rw.getRouteSubPath())

		// Find pod by backtracking route -> service -> pod
		healthEndpoint, exists := rw.tryGetHealthEndpointFromRoute()

		// Health endpoint from pod successful
		if exists {
			u.Path = path.Join(u.Path, healthEndpoint)
		} else { // Try to get annotation and set

			// Annotation for health Endpoint exists
			if value, ok := annotations[constants.MonitorHealthAnnotation]; ok {
				u.Path = path.Join(u.Path, value)
			}
		}
	}

	return u.String()
}
