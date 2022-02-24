package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	storagev1alpha1 "github.com/onmetal/onmetal-api/apis/storage/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const volumeClaimFieldOwner = client.FieldOwner("storage.onmetal.de/volumeclaim")

func (s *service) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	fmt.Println("")
	fmt.Println("create volume request received with volume name", req.GetName())
	capacity := req.GetCapacityRange()
	csiVolResp := &csi.CreateVolumeResponse{}
	volBytes, sVolSize, err := validateVolumeSize(capacity)
	if err != nil {
		fmt.Println("err", err)
		return csiVolResp, status.Errorf(codes.Internal, err.Error())
	}
	params := req.GetParameters()
	fstype := params["fstype"]
	storage_class := params["storage_class_name"]

	vol := &Volume{
		ID:          req.GetName(),
		Name:        req.GetName(),
		StoragePool: req.GetParameters()["storage_pool"],
		Size:        volBytes,
		FsType:      fstype,
	}
	volResp := s.getCsiVolume(vol, req)
	csiVolResp.Volume = volResp

	volumeClaim := &storagev1alpha1.VolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: storagev1alpha1.GroupVersion.String(),
			Kind:       "VolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "onmetal-csi",
			Name:      req.GetName() + "-claim",
		},
		Spec: storagev1alpha1.VolumeClaimSpec{
			Resources: map[corev1.ResourceName]resource.Quantity{
				"storage": resource.MustParse(sVolSize),
			},
			Selector: &metav1.LabelSelector{},
			StorageClassRef: corev1.LocalObjectReference{
				Name: storage_class,
			},
		},
	}

	fmt.Println("create/update volume claim: ", volumeClaim.Name)
	if err := s.parentClient.Patch(ctx, volumeClaim, client.Apply, volumeClaimFieldOwner); err != nil {
		fmt.Println("error while create/update volumeclaim ", err)
		return csiVolResp, status.Errorf(codes.Internal, err.Error())
	}
	volumeClaimKey := types.NamespacedName{
		Namespace: volumeClaim.Namespace,
		Name:      volumeClaim.Name,
	}
	if volumeClaim.Status.Phase != storagev1alpha1.VolumeClaimBound {
		time.Sleep(time.Second * 5)
		vc := &storagev1alpha1.VolumeClaim{}
		err = s.parentClient.Get(ctx, client.ObjectKey{Name: volumeClaim.Name, Namespace: volumeClaim.Namespace}, vc)
		if err != nil && !apierrors.IsNotFound(err) {
			fmt.Printf("could not get voumeclaim with name %s,namespace %s, error:%v", volumeClaimKey.Name, volumeClaimKey.Namespace, err)
			fmt.Println("")
			return csiVolResp, status.Errorf(codes.Internal, err.Error())
		}
		if vc.Status.Phase != storagev1alpha1.VolumeClaimBound {
			fmt.Println("volume claim is not satishfied")
			// TODO
			// err = s.parentClient.Delete(ctx, vc)
			// if err != nil {
			// 	fmt.Printf("unable to delete voumeclaim with name %s,namespace %s, error:%v", volumeClaimKey.Name, volumeClaimKey.Namespace, err)
			// 	fmt.Println("")
			// 	return csiVolResp, status.Errorf(codes.Internal, err.Error())
			// }
			return csiVolResp, status.Errorf(codes.Internal, "unable to process request for volume: ", req.GetName(), "claim pahse", vc.Status.Phase)
		}
	}
	return csiVolResp, nil
}

func (s *service) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	fmt.Println("delete volume request received with volume ID", req.GetVolumeId())
	deleteResponce := &csi.DeleteVolumeResponse{}
	volumeClaimKey := types.NamespacedName{
		Namespace: "onmetal-csi",
		Name:      req.GetVolumeId() + "-claim",
	}
	vc := &storagev1alpha1.VolumeClaim{}
	err := s.parentClient.Get(ctx, volumeClaimKey, vc)
	if err != nil && !apierrors.IsNotFound(err) {
		fmt.Printf("could not get voumeclaim with name %s,namespace %s, error:%v", volumeClaimKey.Name, volumeClaimKey.Namespace, err)
		fmt.Println("")
		return deleteResponce, status.Errorf(codes.Internal, err.Error())
	}
	if apierrors.IsNotFound(err) {
		fmt.Println("volumeclaim is already been deleted")
		return deleteResponce, nil
	}
	if vc != nil {
		err = s.parentClient.Delete(ctx, vc)
		if err != nil {
			fmt.Printf("unable to delete voumeclaim with name %s,namespace %s, error:%v", volumeClaimKey.Name, volumeClaimKey.Namespace, err)
			fmt.Println("")
			return deleteResponce, status.Errorf(codes.Internal, err.Error())
		}
		fmt.Println("deleted volumeclaim ", volumeClaimKey.Name)
	}
	return deleteResponce, nil
}

func (s *service) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (controlePublishResponce *csi.ControllerPublishVolumeResponse, err error) {
	fmt.Printf("request recieved to publish volume %s at node %s", req.GetVolumeId(), req.GetNodeId())
	volCtx := make(map[string]string)
	volCtx["node_id"] = req.GetNodeId()
	volCtx["volume_id"] = req.GetVolumeId()
	fmt.Println("")
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: volCtx,
	}, nil
}

func (s *service) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	fmt.Printf("request recieved to un-publish volume %s at node %s", req.GetVolumeId(), req.GetNodeId())
	controleUnPublishResponce := &csi.ControllerUnpublishVolumeResponse{}
	return controleUnPublishResponce, nil
}

func (s *service) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return &csi.ListVolumesResponse{}, nil
}

func (s *service) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (s *service) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (capacityResponse *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}

func (s *service) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (createSnapshot *csi.CreateSnapshotResponse, err error) {
	return createSnapshot, nil
}

func (s *service) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	return deleteSnapshot, nil
}

func (s *service) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	return expandVolume, nil
}

func (s *service) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return &csi.ValidateVolumeCapabilitiesResponse{}, nil
}

func (s *service) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_GET_CAPACITY,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

type Volume struct {
	ID            string
	Name          string
	StoragePool   string
	CreatedAt     int64
	Size          int64
	FsType        string
	ProvisionType string
}

func (s *service) getCsiVolume(vol *Volume, req *csi.CreateVolumeRequest) *csi.Volume {
	volCtx := map[string]string{
		"volume_id":      vol.ID,
		"volume_name":    vol.Name,
		"storage_pool":   vol.StoragePool,
		"creation_time":  time.Unix(int64(vol.CreatedAt), 0).String(),
		"fstype":         vol.FsType,
		"provision_type": vol.ProvisionType,
	}
	csiVol := &csi.Volume{
		VolumeId:      vol.ID,
		CapacityBytes: vol.Size,
		VolumeContext: volCtx,
		ContentSource: req.GetVolumeContentSource(),
	}
	return csiVol
}

func validateVolumeSize(caprange *csi.CapacityRange) (int64, string, error) {
	requiredVolSize := int64(caprange.GetRequiredBytes())
	allowedMaxVolSize := int64(caprange.GetLimitBytes())
	if requiredVolSize < 0 || allowedMaxVolSize < 0 {
		return 0, "", errors.New("not valid volume size")
	}

	var bytesofKiB int64 = 1024
	var kiBytesofGiB int64 = 1024 * 1024
	var bytesofGiB int64 = kiBytesofGiB * bytesofKiB
	var MinVolumeSize int64 = 1 * bytesofGiB
	fmt.Println("req size", requiredVolSize)
	if requiredVolSize == 0 {
		requiredVolSize = MinVolumeSize
	}

	var (
		sizeinGB   int64
		sizeinByte int64
	)

	sizeinGB = requiredVolSize / bytesofGiB
	fmt.Println("sizeinGB", sizeinGB)
	if sizeinGB == 0 {
		fmt.Println("Volumen Minimum capacity should be greater 1 GB")
		sizeinGB = 1
	}

	sizeinByte = sizeinGB * bytesofGiB
	fmt.Println("sizeinByte", sizeinByte)
	if allowedMaxVolSize != 0 {
		if sizeinByte > allowedMaxVolSize {
			return 0, "", errors.New("volume size is out of allowed limit")
		}
	}
	strsize := strconv.FormatInt(sizeinGB, 10) + "Gi"
	fmt.Println("strsize", strsize)
	return sizeinByte, strsize, nil
}
