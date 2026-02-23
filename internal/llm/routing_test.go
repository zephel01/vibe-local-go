package llm

import (
	"context"
	"testing"
	"time"
)

func TestNewModelRouter(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	if router.mainClient != mainClient {
		t.Error("mainClient not set correctly")
	}
	if router.sidecarClient != sidecarClient {
		t.Error("sidecarClient not set correctly")
	}
	if router.mainModel != "main-model" {
		t.Error("mainModel not set correctly")
	}
	if router.sidecarModel != "sidecar-model" {
		t.Error("sidecarModel not set correctly")
	}
	if router.useSidecar != false {
		t.Error("useSidecar should default to false")
	}
	if router.sidecarLoaded != false {
		t.Error("sidecarLoaded should default to false")
	}
}

func TestModelRouter_SwitchToMain(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")
	router.SwitchToSidecar()
	router.SwitchToMain()

	if router.useSidecar != false {
		t.Error("useSidecar should be false after SwitchToMain")
	}
	if router.GetActiveModel() != "main-model" {
		t.Errorf("GetActiveModel() = %v, want main-model", router.GetActiveModel())
	}
}

func TestModelRouter_SwitchToSidecar(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")
	router.SwitchToSidecar()

	if router.useSidecar != true {
		t.Error("useSidecar should be true after SwitchToSidecar")
	}
	if router.GetActiveModel() != "sidecar-model" {
		t.Errorf("GetActiveModel() = %v, want sidecar-model", router.GetActiveModel())
	}
}

func TestModelRouter_GetActiveModel(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	// Test main model
	model := router.GetActiveModel()
	if model != "main-model" {
		t.Errorf("GetActiveModel() = %v, want main-model", model)
	}

	// Test sidecar model
	router.SwitchToSidecar()
	model = router.GetActiveModel()
	if model != "sidecar-model" {
		t.Errorf("GetActiveModel() = %v, want sidecar-model", model)
	}
}

func TestModelRouter_GetActiveClient(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	// Test main client
	client := router.GetActiveClient()
	if client != mainClient {
		t.Error("GetActiveClient() returned wrong client")
	}

	// Test sidecar client
	router.SwitchToSidecar()
	client = router.GetActiveClient()
	if client != sidecarClient {
		t.Error("GetActiveClient() returned wrong client")
	}
}

func TestModelRouter_AutoSelectModel(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	tests := []struct {
		taskType  string
		wantSidecar bool
	}{
		{"code_generation", false},
		{"code_review", true},
		{"documentation", true},
		{"quick_edit", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.taskType, func(t *testing.T) {
			router.AutoSelectModel(tt.taskType)
			if router.useSidecar != tt.wantSidecar {
				t.Errorf("AutoSelectModel(%s) useSidecar = %v, want %v", tt.taskType, router.useSidecar, tt.wantSidecar)
			}
		})
	}
}

func TestModelRouter_AutoSelectModel_NoSidecar(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	router := NewModelRouter(mainClient, nil, "main-model", "")

	tests := []string{"code_review", "documentation"}
	for _, taskType := range tests {
		t.Run(taskType+"_no_sidecar", func(t *testing.T) {
			router.AutoSelectModel(taskType)
			if router.useSidecar != false {
				t.Errorf("AutoSelectModel(%s) should not switch to sidecar when not available", taskType)
			}
		})
	}
}

func TestModelRouter_SelectModelByMemory(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "", "")

	tests := []struct {
		name      string
		memoryGB  float64
		wantModel string
	}{
		{"Tier A", 256, "qwen2.5-72b-instruct"},
		{"Tier B", 96, "llama3.1-70b-instruct"},
		{"Tier C high", 32, "qwen2.5-32b-instruct"},
		{"Tier C mid", 16, "llama3.1-8b-instruct"},
		{"Tier D", 8, "llama3.2-3b-instruct"},
		{"Tier E", 4, "qwen2.5-1.5b-instruct"},
		{"Below Tier E", 2, "qwen2.5-1.5b-instruct"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := router.SelectModelByMemory(tt.memoryGB)
			if model != tt.wantModel {
				t.Errorf("SelectModelByMemory(%v) = %v, want %v", tt.memoryGB, model, tt.wantModel)
			}
		})
	}
}

func TestModelRouter_GetModelTier(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "", "")

	tests := []struct {
		model string
		tier  string
	}{
		{"qwen2.5-72b-instruct", "A"},
		{"llama3.1-70b-instruct", "B"},
		{"qwen2.5-32b-instruct", "C"},
		{"llama3.1-8b-instruct", "D"},
		{"llama3.2-3b-instruct", "E"},
		{"qwen2.5-1.5b-instruct", "E"},
		{"qwen2.5-1.7b-instruct", "E"},
		{"unknown-model", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			tier := router.GetModelTier(tt.model)
			if tier != tt.tier {
				t.Errorf("GetModelTier(%s) = %v, want %v", tt.model, tier, tt.tier)
			}
		})
	}
}

func TestModelRouter_GetStatus(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")

	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	status := router.GetStatus()

	if status.MainModel != "main-model" {
		t.Errorf("MainModel = %v, want main-model", status.MainModel)
	}
	if status.SidecarModel != "sidecar-model" {
		t.Errorf("SidecarModel = %v, want sidecar-model", status.SidecarModel)
	}
	if status.ActiveModel != "main-model" {
		t.Errorf("ActiveModel = %v, want main-model", status.ActiveModel)
	}
	if status.UsingSidecar != false {
		t.Errorf("UsingSidecar = %v, want false", status.UsingSidecar)
	}
	if status.SidecarLoaded != false {
		t.Errorf("SidecarLoaded = %v, want false", status.SidecarLoaded)
	}

	// Test after switching
	router.SwitchToSidecar()
	status = router.GetStatus()
	if status.ActiveModel != "sidecar-model" {
		t.Errorf("ActiveModel = %v, want sidecar-model", status.ActiveModel)
	}
	if status.UsingSidecar != true {
		t.Errorf("UsingSidecar = %v, want true", status.UsingSidecar)
	}
}

func TestModelRouter_PreloadSidecar_NoSidecar(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	router := NewModelRouter(mainClient, nil, "main-model", "")

	ctx := context.Background()
	err := router.PreloadSidecar(ctx)
	if err != nil {
		t.Errorf("PreloadSidecar() with no sidecar should not error, got %v", err)
	}
}

func TestModelRouter_PreloadSidecar_AlreadyLoaded(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")
	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	// Mark as loaded
	router.sidecarLoaded = true

	ctx := context.Background()
	err := router.PreloadSidecar(ctx)
	if err != nil {
		t.Errorf("PreloadSidecar() already loaded should not error, got %v", err)
	}
}

func TestModelRouter_SwapModelHot_NoChange(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")
	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	ctx := context.Background()
	err := router.SwapModelHot(ctx, false)
	if err != nil {
		t.Errorf("SwapModelHot() no change should not error, got %v", err)
	}
	if router.useSidecar != false {
		t.Error("useSidecar should remain false")
	}
}

func TestModelRouter_KeepAliveAlive_Cancel(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")
	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	ctx, cancel := context.WithCancel(context.Background())
	go router.KeepAliveAlive(ctx, 10*time.Millisecond)

	// Cancel after short delay
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// If we reach here, the goroutine was cancelled successfully
}

func TestModelRouter_ConcurrentAccess(t *testing.T) {
	mainClient := NewClient("http://localhost:11434")
	sidecarClient := NewClient("http://localhost:11434")
	router := NewModelRouter(mainClient, sidecarClient, "main-model", "sidecar-model")

	done := make(chan bool)

	// Run multiple concurrent operations
	for i := 0; i < 10; i++ {
		go func() {
			router.GetActiveModel()
			router.GetActiveClient()
			router.GetStatus()
			done <- true
		}()
		go func() {
			if i%2 == 0 {
				router.SwitchToMain()
			} else {
				router.SwitchToSidecar()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}
}
