package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"

	eirinix "github.com/SUSE/eirinix"
	"go.uber.org/zap"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// VolumeMount is a volume assigned to the app
type VolumeMount struct {
	ContainerDir string `json:"container_dir"`
	DeviceType   string `json:"device_type"`
	Mode         string `json:"mode"`
}

// Credentials is containing the volume id assigned to the pod
type Credentials struct {
	// VolumeID represents a Persistent Volume Claim
	VolumeID string `json:"volume_id"`
}

// VcapService contains the service configuration. We look only at volume mounts here
type VcapService struct {
	Credentials  Credentials   `json:"credentials"`
	VolumeMounts []VolumeMount `json:"volume_mounts"`
}

// VcapServices represent the VCAP_SERVICE structure, specific to this extension
type VcapServices struct {
	ServiceMap []VcapService `json:"eirini-persi"`
}

// Extension changes pod definitions
type Extension struct{ Logger *zap.SugaredLogger }

func containsContainerMount(containermounts []corev1.VolumeMount, mount string) bool {
	for _, m := range containermounts {
		if m.Name == mount {
			return true
		}
	}
	return false
}

// AppendMounts appends volumes that are specified in VCAP_SERVICES to the pod and to the container given as arguments
func (s VcapServices) AppendMounts(patchedSet *appv1.StatefulSet, c *corev1.Container) {
	for _, volumeService := range s.ServiceMap {
		for _, volumeMount := range volumeService.VolumeMounts {
			if !containsContainerMount(c.VolumeMounts, volumeService.Credentials.VolumeID) {
				//				patchedPod.Spec.Volumes = append(patchedPod.Spec.Volumes, corev1.Volume{
				//					Name: volumeService.Credentials.VolumeID,
				//					VolumeSource: corev1.VolumeSource{
				//						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				//							ClaimName: volumeService.Credentials.VolumeID,
				//						},
				//					},
				//				})
				quantity, err := resource.ParseQuantity("1Gi")
				if err != nil {
					panic(err.Error())
				}
				storageClass := "standard"
				patchedSet.Spec.VolumeClaimTemplates = append(patchedSet.Spec.VolumeClaimTemplates, corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: volumeService.Credentials.VolumeID,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": quantity,
							},
						},
						StorageClassName: &storageClass,
					},
				})

				c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
					Name:      volumeService.Credentials.VolumeID,
					MountPath: volumeMount.ContainerDir,
				})
				u := int64(0)
				patchedSet.Spec.Template.Spec.InitContainers =
					append(patchedSet.Spec.Template.Spec.InitContainers, corev1.Container{
						SecurityContext: &corev1.SecurityContext{RunAsUser: &u},
						Name:            fmt.Sprintf("eirini-persi-%s", volumeService.Credentials.VolumeID),
						Image:           c.Image,
						VolumeMounts:    c.VolumeMounts,
						Command: []string{
							"sh",
							"-c",
							fmt.Sprintf("chown -R vcap:vcap %s", volumeMount.ContainerDir),
						},
					})
			}
		}
	}
}

// MountVcapVolumes alters the pod given as argument with the required volumes mounted
func (ext *Extension) MountVcapVolumes(patchedSet *appv1.StatefulSet) error {
	for i := range patchedSet.Spec.Template.Spec.Containers {
		c := &patchedSet.Spec.Template.Spec.Containers[i]
		for _, env := range c.Env {
			if env.Name != "VCAP_SERVICES" {
				continue
			}
			ext.Logger.Debug("Appending volumes to the Eirini App")

			var services VcapServices
			err := json.Unmarshal([]byte(env.Value), &services)
			if err != nil {
				return err
			}
			services.AppendMounts(patchedSet, c)
			break
		}
	}
	return nil
}

// New returns the persi extension
func New() eirinix.Extension {
	return &Extension{}
}

// Handle manages volume claims for ExtendedStatefulSet pods
func (ext *Extension) Handle(ctx context.Context, eiriniManager eirinix.Manager, set *appv1.StatefulSet, req types.Request) types.Response {

	if set == nil {
		return admission.ErrorResponse(http.StatusBadRequest, errors.New("No set could be decoded from the request"))
	}

	_, file, _, _ := runtime.Caller(0)
	log := eiriniManager.GetLogger().Named(file)

	ext.Logger = log
	setCopy := set.DeepCopy()
	log.Debugf("Handling webhook request for POD: %s (%s)", setCopy.Name, setCopy.Namespace)

	err := ext.MountVcapVolumes(setCopy)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	return admission.PatchResponse(set, setCopy)
}
