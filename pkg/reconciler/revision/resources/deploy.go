/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resources

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"context"
	"log"

	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/ptr"
	"knative.dev/serving/pkg/apis/autoscaling"
	"knative.dev/serving/pkg/apis/serving"
	v1 "knative.dev/serving/pkg/apis/serving/v1"
	"knative.dev/serving/pkg/networking"
	"knative.dev/serving/pkg/queue"
	"knative.dev/serving/pkg/reconciler/revision/config"
	"knative.dev/serving/pkg/reconciler/revision/resources/names"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/dynamic"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiconfig "knative.dev/serving/pkg/apis/config"
)

// Structs to unmarshal the YAML content
type CustomSchedulerSpec struct {
    SchedulerName string `yaml:"schedulerName"`
}

type CustomScheduler struct {
    APIVersion string              `yaml:"apiVersion"`
    Kind       string              `yaml:"kind"`
    Metadata   metav1.ObjectMeta   `yaml:"metadata"`
    Spec       CustomSchedulerSpec `yaml:"spec"`
}

type ConfigMapData struct {
    SchedulerName      string `yaml:"schedulerName"`
    SchedulerNamespace string `yaml:"schedulerNamespace"`
    CustomMetrics      string `yaml:"customMetrics"`
    Parameters         string `yaml:"parameters"`
}

type ConfigMap struct {
    APIVersion string            `yaml:"apiVersion"`
    Kind       string            `yaml:"kind"`
    Metadata   metav1.ObjectMeta `yaml:"metadata"`
    Data       ConfigMapData     `yaml:"data"`
}

const certVolumeName = "server-certs"

var (
	varLogVolume = corev1.Volume{
		Name: "knative-var-log",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	varLogVolumeMount = corev1.VolumeMount{
		Name:        varLogVolume.Name,
		MountPath:   "/var/log",
		SubPathExpr: "$(K_INTERNAL_POD_NAMESPACE)_$(K_INTERNAL_POD_NAME)_",
	}

	//nolint:gosec // Volume, not hardcoded credentials
	varTokenVolume = corev1.Volume{
		Name: "knative-token-volume",
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: []corev1.VolumeProjection{},
			},
		},
	}

	varCertVolumeMount = corev1.VolumeMount{
		MountPath: queue.CertDirectory,
		Name:      certVolumeName,
		ReadOnly:  true,
	}

	//nolint:gosec // VolumeMount, not hardcoded credentials
	varTokenVolumeMount = corev1.VolumeMount{
		Name:      varTokenVolume.Name,
		MountPath: queue.TokenDirectory,
	}

	varPodInfoVolume = corev1.Volume{
		Name: "pod-info",
		VolumeSource: corev1.VolumeSource{
			DownwardAPI: &corev1.DownwardAPIVolumeSource{
				Items: []corev1.DownwardAPIVolumeFile{{
					Path: queue.PodInfoAnnotationsFilename,
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.annotations",
					},
				}},
			},
		},
	}

	varPodInfoVolumeMount = corev1.VolumeMount{
		Name:      varPodInfoVolume.Name,
		MountPath: queue.PodInfoDirectory,
		ReadOnly:  true,
	}

	// This PreStop hook is actually calling an endpoint on the queue-proxy
	// because of the way PreStop hooks are called by kubelet. We use this
	// to block the user-container from exiting before the queue-proxy is ready
	// to exit so we can guarantee that there are no more requests in flight.
	userLifecycle = &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Port: intstr.FromInt(networking.QueueAdminPort),
				Path: queue.RequestQueueDrainPath,
			},
		},
	}
)

func addToken(tokenVolume *corev1.Volume, filename string, audience string, expiry *int64) {
	if filename == "" || audience == "" {
		return
	}
	volumeProjection := &corev1.VolumeProjection{
		ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
			ExpirationSeconds: expiry,
			Path:              filename,
			Audience:          audience,
		},
	}
	tokenVolume.VolumeSource.Projected.Sources = append(tokenVolume.VolumeSource.Projected.Sources, *volumeProjection)
}

func certVolume(secret string) corev1.Volume {
	return corev1.Volume{
		Name: certVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secret,
			},
		},
	}
}

func rewriteUserLivenessProbe(p *corev1.Probe, userPort int) {
	if p == nil {
		return
	}
	switch {
	case p.HTTPGet != nil:
		p.HTTPGet.Port = intstr.FromInt(userPort)
	case p.TCPSocket != nil:
		p.TCPSocket.Port = intstr.FromInt(userPort)
	}
}

func makePodSpec(rev *v1.Revision, cfg *config.Config) (*corev1.PodSpec, error) {
	queueContainer, err := makeQueueContainer(rev, cfg)
	tokenVolume := varTokenVolume.DeepCopy()

	if err != nil {
		return nil, fmt.Errorf("failed to create queue-proxy container: %w", err)
	}

	var extraVolumes []corev1.Volume

	podInfoFeature, podInfoExists := rev.Annotations[apiconfig.QueueProxyPodInfoFeatureKey]

	if cfg.Features.QueueProxyMountPodInfo == apiconfig.Enabled ||
		(cfg.Features.QueueProxyMountPodInfo == apiconfig.Allowed &&
			podInfoExists &&
			strings.EqualFold(podInfoFeature, string(apiconfig.Enabled))) {
		queueContainer.VolumeMounts = append(queueContainer.VolumeMounts, varPodInfoVolumeMount)
		extraVolumes = append(extraVolumes, varPodInfoVolume)
	}

	audiences := make([]string, 0, len(cfg.Deployment.QueueSidecarTokenAudiences))
	for k := range cfg.Deployment.QueueSidecarTokenAudiences {
		audiences = append(audiences, k)
	}
	sort.Strings(audiences)
	for _, aud := range audiences {
		// add token for audience <aud> under filename <aud>
		addToken(tokenVolume, aud, aud, ptr.Int64(3600))
	}

	if len(tokenVolume.VolumeSource.Projected.Sources) > 0 {
		queueContainer.VolumeMounts = append(queueContainer.VolumeMounts, varTokenVolumeMount)
		extraVolumes = append(extraVolumes, *tokenVolume)
	}

	if cfg.Network.SystemInternalTLSEnabled() {
		queueContainer.VolumeMounts = append(queueContainer.VolumeMounts, varCertVolumeMount)
		extraVolumes = append(extraVolumes, certVolume(networking.ServingCertName))
	}

	schedulerName, err := determineSchedulerName(rev.Namespace)
    if err != nil {
        log.Printf("deploy.go: Error determining scheduler name: %v", err)
        schedulerName = ""
    }

	// Check if a CustomScheduler is deployed in the cluster
	// If it is, read the schedulerName
		// Check if there is a ConfigMap with the name scheduler-config
		// If there is, read the schedulerName and namespace from the ConfigMap
			// Look for a pod with the name schedulerName in the namespace
			// If it exists, use the schedulerName
				// schedulerName = defaultCustomSchedulerName
			// If it does not exist, use the default Kubernetes scheduler
		// If there is not, use the defaultCustomSchedulerName
	// If it is not, use the default Kubernetes scheduler

	podSpec := BuildPodSpec(rev, append(BuildUserContainers(rev), *queueContainer), cfg)
	podSpec.Volumes = append(podSpec.Volumes, extraVolumes...)
	podSpec.SchedulerName = schedulerName

	if cfg.Observability.EnableVarLogCollection {
		podSpec.Volumes = append(podSpec.Volumes, varLogVolume)

		for i, container := range podSpec.Containers {
			if container.Name == QueueContainerName {
				continue
			}

			varLogMount := varLogVolumeMount.DeepCopy()
			varLogMount.SubPathExpr += container.Name
			container.VolumeMounts = append(container.VolumeMounts, *varLogMount)
			container.Env = append(container.Env, buildVarLogSubpathEnvs()...)

			podSpec.Containers[i] = container
		}
	}

	return podSpec, nil
}

func getCustomSchedulerName(dynamicClient dynamic.Interface, namespace string) (string, error) {
    log.Println("Attempting to get the active CustomScheduler...")

    gvr := schema.GroupVersionResource{
        Group:    "serving.local.dev",
        Version:  "v1alpha1",
        Resource: "customschedulers",
    }

    customSchedulerList, err := dynamicClient.Resource(gvr).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
    if err != nil {
        log.Printf("deploy.go: Error getting CustomScheduler list: %v", err)
        return "", err
    }

    if len(customSchedulerList.Items) == 0 {
        log.Println("No active CustomScheduler found.")
        return "", nil
    }

    // Assuming only one active CustomScheduler
    customScheduler := customSchedulerList.Items[0]
    schedulerName, found, err := unstructured.NestedString(customScheduler.Object, "spec", "schedulerName")
    if err != nil || !found {
        log.Printf("deploy.go: Error getting schedulerName from CustomScheduler: %v", err)
        return "", err
    }

    log.Printf("deploy.go: Active CustomScheduler found with schedulerName: %s", schedulerName)
    return schedulerName, nil
}

func getSchedulerConfig(kubeClient kubernetes.Interface) (string, string, error) {
    log.Println("Attempting to get the scheduler-config ConfigMap...")
    configMap, err := kubeClient.CoreV1().ConfigMaps("default").Get(context.TODO(), "scheduler-config", metav1.GetOptions{})
    if err != nil {
        log.Printf("deploy.go: Error getting scheduler-config ConfigMap: %v", err)
        return "", "", err
    }

    schedulerName := configMap.Data["schedulerName"]
    schedulerNamespace := configMap.Data["schedulerNamespace"]
    log.Printf("deploy.go: Scheduler config found: schedulerName=%s, schedulerNamespace=%s", schedulerName, schedulerNamespace)
    return schedulerName, schedulerNamespace, nil
}

func isSchedulerDeploymentRunning(kubeClient kubernetes.Interface, schedulerName, schedulerNamespace string) (bool, error) {
    log.Printf("deploy.go: Checking if the scheduler deployment %s in namespace %s is running...", schedulerName, schedulerNamespace)
    pods, err := kubeClient.CoreV1().Pods(schedulerNamespace).List(context.TODO(), metav1.ListOptions{
        LabelSelector: fmt.Sprintf("app=%s", schedulerName),
    })
    if err != nil {
        log.Printf("deploy.go: Error listing pods for scheduler deployment: %v", err)
        return false, err
    }

    if len(pods.Items) == 0 {
        log.Printf("deploy.go: No pods found for scheduler deployment %s in namespace %s", schedulerName, schedulerNamespace)
        return false, nil
    }

    for _, pod := range pods.Items {
        if pod.Status.Phase == corev1.PodRunning {
            log.Printf("deploy.go: Scheduler deployment %s is running", schedulerName)
            return true, nil
        }
    }

    log.Printf("deploy.go: Scheduler deployment %s is not running", schedulerName)
    return false, nil
}

func determineSchedulerName(namespace string) (string, error) {
    config, err := rest.InClusterConfig()
    if err != nil {
        return "", fmt.Errorf("failed to create in-cluster config: %w", err)
    }

    kubeClient, err := kubernetes.NewForConfig(config)
    if err != nil {
        return "", fmt.Errorf("failed to create kubernetes client: %w", err)
    }

    dynamicClient, err := dynamic.NewForConfig(config)
    if err != nil {
        return "", fmt.Errorf("failed to create dynamic client: %w", err)
    }

    schedulerName, err := getCustomSchedulerName(dynamicClient, namespace)
    if err != nil || schedulerName == "" {
        log.Println("Using default scheduler.")
        return "", err
    }

    extSchedulerName, extSchedulerNamespace, err := getSchedulerConfig(kubeClient)
    if err != nil {
        log.Println("Using default scheduler.")
        return "", err
    }

    isRunning, err := isSchedulerDeploymentRunning(kubeClient, extSchedulerName, extSchedulerNamespace)
    if err != nil || !isRunning {
        log.Println("Using default scheduler.")
        return "", err
    }

    return schedulerName, nil
}


// BuildUserContainers makes an array of containers from the Revision template.
func BuildUserContainers(rev *v1.Revision) []corev1.Container {
	containers := make([]corev1.Container, 0, len(rev.Spec.PodSpec.Containers))
	for i := range rev.Spec.PodSpec.Containers {
		var container corev1.Container
		if len(rev.Spec.PodSpec.Containers[i].Ports) != 0 || len(rev.Spec.PodSpec.Containers) == 1 {
			container = makeServingContainer(*rev.Spec.PodSpec.Containers[i].DeepCopy(), rev)
		} else {
			container = makeContainer(*rev.Spec.PodSpec.Containers[i].DeepCopy(), rev)
		}
		// The below logic is safe because the image digests in Status.ContainerStatus will have been resolved
		// before this method is called. We check for an empty array here because the method can also be
		// called during DryRun, where ContainerStatuses will not yet have been resolved.
		if len(rev.Status.ContainerStatuses) != 0 {
			if rev.Status.ContainerStatuses[i].ImageDigest != "" {
				container.Image = rev.Status.ContainerStatuses[i].ImageDigest
			}
		}
		containers = append(containers, container)
	}
	return containers
}

func makeContainer(container corev1.Container, rev *v1.Revision) corev1.Container {
	// Adding or removing an overwritten corev1.Container field here? Don't forget to
	// update the fieldmasks / validations in pkg/apis/serving
	container.Lifecycle = userLifecycle
	container.Env = append(container.Env, getKnativeEnvVar(rev)...)

	// Explicitly disable stdin and tty allocation
	container.Stdin = false
	container.TTY = false
	if container.TerminationMessagePolicy == "" {
		container.TerminationMessagePolicy = corev1.TerminationMessageFallbackToLogsOnError
	}

	if container.ReadinessProbe != nil {
		if container.ReadinessProbe.HTTPGet != nil || container.ReadinessProbe.TCPSocket != nil || container.ReadinessProbe.GRPC != nil {
			// HTTP, TCP and gRPC ReadinessProbes are executed by the queue-proxy directly against the
			// container instead of via kubelet.
			container.ReadinessProbe = nil
		}
	}

	return container
}

func makeServingContainer(servingContainer corev1.Container, rev *v1.Revision) corev1.Container {
	userPort := getUserPort(rev)
	userPortStr := strconv.Itoa(int(userPort))
	// Replacement is safe as only up to a single port is allowed on the Revision
	servingContainer.Ports = buildContainerPorts(userPort)
	servingContainer.Env = append(servingContainer.Env, buildUserPortEnv(userPortStr))
	container := makeContainer(servingContainer, rev)
	// If the user provides a liveness probe, we should rewrite in the port on the user-container for them.
	rewriteUserLivenessProbe(container.LivenessProbe, int(userPort))
	return container
}

// BuildPodSpec creates a PodSpec from the given revision and containers.
// cfg can be passed as nil if not within revision reconciliation context.
func BuildPodSpec(rev *v1.Revision, containers []corev1.Container, cfg *config.Config) *corev1.PodSpec {
	pod := rev.Spec.PodSpec.DeepCopy()
	pod.Containers = containers
	pod.TerminationGracePeriodSeconds = rev.Spec.TimeoutSeconds
	if cfg != nil && pod.EnableServiceLinks == nil {
		pod.EnableServiceLinks = cfg.Defaults.EnableServiceLinks
	}
	return pod
}

func getUserPort(rev *v1.Revision) int32 {
	ports := rev.Spec.GetContainer().Ports

	if len(ports) > 0 && ports[0].ContainerPort != 0 {
		return ports[0].ContainerPort
	}

	return v1.DefaultUserPort
}

func buildContainerPorts(userPort int32) []corev1.ContainerPort {
	return []corev1.ContainerPort{{
		Name:          v1.UserPortName,
		ContainerPort: userPort,
	}}
}

func buildVarLogSubpathEnvs() []corev1.EnvVar {
	return []corev1.EnvVar{{
		Name: "K_INTERNAL_POD_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.name",
			},
		},
	}, {
		Name: "K_INTERNAL_POD_NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.namespace",
			},
		},
	}}
}

func buildUserPortEnv(userPort string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  "PORT",
		Value: userPort,
	}
}

// MakeDeployment constructs a K8s Deployment resource from a revision.
func MakeDeployment(rev *v1.Revision, cfg *config.Config) (*appsv1.Deployment, error) {
	podSpec, err := makePodSpec(rev, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create PodSpec: %w", err)
	}

	replicaCount := cfg.Autoscaler.InitialScale
	_, ann, found := autoscaling.InitialScaleAnnotation.Get(rev.Annotations)
	if found {
		// Ignore errors and no error checking because already validated in webhook.
		rc, _ := strconv.ParseInt(ann, 10, 32)
		replicaCount = int32(rc)
	}

	progressDeadline := int32(cfg.Deployment.ProgressDeadline.Seconds())
	_, pdAnn, pdFound := serving.ProgressDeadlineAnnotation.Get(rev.Annotations)
	if pdFound {
		// Ignore errors and no error checking because already validated in webhook.
		pd, _ := time.ParseDuration(pdAnn)
		progressDeadline = int32(pd.Seconds())
	}

	labels := makeLabels(rev)
	anns := makeAnnotations(rev)

	// Slowly but steadily roll the deployment out, to have the least possible impact.
	maxUnavailable := intstr.FromInt(0)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.Deployment(rev),
			Namespace:       rev.Namespace,
			Labels:          labels,
			Annotations:     anns,
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(rev)},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:                ptr.Int32(replicaCount),
			Selector:                makeSelector(rev),
			ProgressDeadlineSeconds: ptr.Int32(progressDeadline),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: anns,
				},
				Spec: *podSpec,
			},
		},
	}, nil
}
