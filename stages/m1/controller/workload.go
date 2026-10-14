package controller

import (
	"strconv"

	platform "github.com/win30221/hugofleet/stages/m1/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	UIDLabel           = "platform.hugofleet.io/application-uid"
	RevisionAnnotation = "platform.hugofleet.io/revision"
)

func ptr[T any](v T) *T { return &v }

func managedMetadata(app *platform.Application, object metav1.Object) {
	labels := object.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[UIDLabel] = string(app.UID)
	labels["app.kubernetes.io/name"] = app.Name
	labels["app.kubernetes.io/managed-by"] = "hugofleet"
	object.SetLabels(labels)
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[RevisionAnnotation] = strconv.FormatInt(app.Spec.Revision, 10)
	object.SetAnnotations(annotations)
}

func deploymentSpec(app *platform.Application) appsv1.DeploymentSpec {
	s := app.Spec
	quantity := func(v platform.ResourceValues) corev1.ResourceList {
		return corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(v.CPU), corev1.ResourceMemory: resource.MustParse(v.Memory)}
	}
	probe := func(path string, period int32) *corev1.Probe {
		return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromString("http"), Scheme: corev1.URISchemeHTTP}},
			PeriodSeconds: period, TimeoutSeconds: 1, SuccessThreshold: 1, FailureThreshold: 3}
	}
	return appsv1.DeploymentSpec{
		Replicas: ptr(s.Replicas), RevisionHistoryLimit: ptr[int32](2), ProgressDeadlineSeconds: ptr[int32](60),
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{UIDLabel: string(app.UID)}},
		Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: ptr(intstr.FromInt32(1)), MaxUnavailable: ptr(intstr.FromInt32(0))}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{UIDLabel: string(app.UID), "app.kubernetes.io/name": app.Name}, Annotations: map[string]string{RevisionAnnotation: strconv.FormatInt(s.Revision, 10)}},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways, DNSPolicy: corev1.DNSClusterFirst, SchedulerName: corev1.DefaultSchedulerName,
				TerminationGracePeriodSeconds: ptr[int64](10), AutomountServiceAccountToken: ptr(false), EnableServiceLinks: ptr(false),
				SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: ptr(true), RunAsUser: ptr[int64](65532), RunAsGroup: ptr[int64](65532), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
				Containers: []corev1.Container{{Name: "app", Image: s.Image, ImagePullPolicy: corev1.PullIfNotPresent,
					Ports:          []corev1.ContainerPort{{Name: "http", ContainerPort: s.Port, Protocol: corev1.ProtocolTCP}},
					Resources:      corev1.ResourceRequirements{Requests: quantity(s.Resources.Requests), Limits: quantity(s.Resources.Limits)},
					ReadinessProbe: probe(s.ReadinessPath, 2), LivenessProbe: probe(s.LivenessPath, 5),
					TerminationMessagePath: corev1.TerminationMessagePathDefault, TerminationMessagePolicy: corev1.TerminationMessageReadFile,
					SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr(false), ReadOnlyRootFilesystem: ptr(true), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}},
				}},
			},
		},
	}
}

// DesiredDeploymentSpec is shared with the Hub's independent rollout check.
// Callers must account for user metadata preserved by the controller.
func DesiredDeploymentSpec(app *platform.Application) appsv1.DeploymentSpec {
	return deploymentSpec(app)
}

// Preserve user metadata, but the entire workload specification is controlled by Application.
func mutateDeployment(app *platform.Application, dep *appsv1.Deployment) {
	managedMetadata(app, dep)
	want := deploymentSpec(app)
	for k, v := range dep.Spec.Template.Labels {
		if _, owned := want.Template.Labels[k]; !owned {
			want.Template.Labels[k] = v
		}
	}
	for k, v := range dep.Spec.Template.Annotations {
		if _, owned := want.Template.Annotations[k]; !owned {
			want.Template.Annotations[k] = v
		}
	}
	dep.Spec = want
}

func mutateService(app *platform.Application, service *corev1.Service) {
	managedMetadata(app, service)
	// Keep allocated clusterIP/clusterIPs and IP-family fields unchanged.
	service.Spec.Type = corev1.ServiceTypeClusterIP
	service.Spec.Selector = map[string]string{UIDLabel: string(app.UID)}
	service.Spec.Ports = []corev1.ServicePort{{Name: "http", Port: app.Spec.Port, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP}}
	service.Spec.ExternalName = ""
	service.Spec.ExternalIPs = nil
	service.Spec.LoadBalancerIP = ""
	service.Spec.LoadBalancerClass = nil
	service.Spec.AllocateLoadBalancerNodePorts = nil
	service.Spec.LoadBalancerSourceRanges = nil
	service.Spec.ExternalTrafficPolicy = ""
	service.Spec.HealthCheckNodePort = 0
	service.Spec.SessionAffinity = corev1.ServiceAffinityNone
	service.Spec.SessionAffinityConfig = nil
	service.Spec.PublishNotReadyAddresses = false
	service.Spec.InternalTrafficPolicy = ptr(corev1.ServiceInternalTrafficPolicyCluster)
}
