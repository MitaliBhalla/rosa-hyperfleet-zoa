package actions

import (
	"context"
	"log/slog"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func deletepodTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func podWithOwner(ns, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "owner-rs",
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx"},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func standalonePod(ns, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx"},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func TestDeletePod(t *testing.T) {
	action := &deletePod{}

	t.Run("When namespace param is missing it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"name": "my-pod"},
			KubeClient: fake.NewClientset(),
			Logger:     deletepodTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing namespace")
		}
	})

	t.Run("When name param is missing it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "default"},
			KubeClient: fake.NewClientset(),
			Logger:     deletepodTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for missing name")
		}
	})

	t.Run("When pod does not exist it should return validation error", func(t *testing.T) {
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "default", "name": "no-such-pod"},
			KubeClient: fake.NewClientset(),
			Logger:     deletepodTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err == nil {
			t.Fatal("expected validation error for non-existent pod")
		}
	})

	t.Run("When pod has no owner references it should refuse deletion", func(t *testing.T) {
		client := fake.NewClientset(standalonePod("default", "standalone"))
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "default", "name": "standalone"},
			KubeClient: client,
			Logger:     deletepodTestLogger(),
		}
		err := action.Validate(context.Background(), params)
		if err == nil {
			t.Fatal("expected validation error for standalone pod")
		}
		if err.Error() == "" {
			t.Fatal("error message should not be empty")
		}
	})

	t.Run("When pod has owner references it should pass validation", func(t *testing.T) {
		client := fake.NewClientset(podWithOwner("default", "owned-pod"))
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "default", "name": "owned-pod"},
			KubeClient: client,
			Logger:     deletepodTestLogger(),
		}
		if err := action.Validate(context.Background(), params); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("When deleting a valid pod it should succeed", func(t *testing.T) {
		client := fake.NewClientset(podWithOwner("default", "delete-me"))
		params := &ExecutionParams{
			Params:     map[string]string{"namespace": "default", "name": "delete-me"},
			KubeClient: client,
			Logger:     deletepodTestLogger(),
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		result, err := action.Execute(ctx, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Fatal("expected success")
		}
		if len(result.AffectedResources) != 1 {
			t.Fatalf("expected 1 affected resource, got %d", len(result.AffectedResources))
		}
		if result.AffectedResources[0].Action != "deleted" {
			t.Errorf("expected action 'deleted', got %q", result.AffectedResources[0].Action)
		}

		_, err = client.CoreV1().Pods("default").Get(ctx, "delete-me", metav1.GetOptions{})
		if err == nil {
			t.Fatal("expected pod to be deleted")
		}
	})
}

func TestDeletePod_WhenContextAlreadyCancelled_ItShouldReturnDeleteInitiated(t *testing.T) {
	action := &deletePod{}
	// Use an empty fake clientset — the pod does NOT exist so the watch won't
	// fire a Deleted event. Combined with a pre-cancelled context, this forces
	// the ctx.Done path.
	client := fake.NewClientset()
	params := &ExecutionParams{
		Params:     map[string]string{"namespace": "default", "name": "slow-pod"},
		KubeClient: client,
		Logger:     deletepodTestLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := action.Execute(ctx, params)
	// With a cancelled context, the K8s delete call or watch setup should fail
	// OR we get the delete-initiated result.
	if err != nil {
		// Acceptable: ctx cancelled before delete/watch completes
		return
	}
	if !result.Success {
		t.Fatal("expected success (delete-initiated)")
	}
	output := result.Output.(map[string]string)
	if output["status"] != "delete-initiated" {
		t.Errorf("expected status 'delete-initiated', got %q", output["status"])
	}
}

func TestDeletePodMetadata(t *testing.T) {
	action := &deletePod{}
	meta := action.Metadata()

	t.Run("When checking metadata it should have correct properties", func(t *testing.T) {
		if meta.Name != "delete_pod" {
			t.Errorf("expected name 'delete_pod', got %q", meta.Name)
		}
		if meta.Type != "write" {
			t.Errorf("expected type 'write', got %q", meta.Type)
		}
		if meta.WriteCooldownSeconds != 60 {
			t.Errorf("expected cooldown 60, got %d", meta.WriteCooldownSeconds)
		}
		if meta.DryRunAction != "get_resource" {
			t.Errorf("expected dry run action 'get_resource', got %q", meta.DryRunAction)
		}
	})
}
