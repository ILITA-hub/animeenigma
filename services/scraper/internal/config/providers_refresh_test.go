package config

import (
	"sync"
	"testing"
)

func TestProvidersConfig_ReplaceIsAtomic(t *testing.T) {
	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "allanime", Status: StatusEnabled}})
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = pc.IsEnabled("allanime")
					_ = pc.Meta("allanime")
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		st := StatusEnabled
		if i%2 != 0 {
			st = StatusDisabled
		}
		pc.Replace([]ProviderMeta{{Name: "allanime", Status: st}})
	}
	pc.Replace([]ProviderMeta{{Name: "allanime", Status: StatusDisabled}})
	close(stop)
	wg.Wait()
	if pc.IsEnabled("allanime") {
		t.Error("expected allanime disabled after final Replace")
	}
}
