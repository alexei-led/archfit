package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/runtime"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// noopRunner returns a RunnerMock that reports no tools and never runs.
func noopRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{}, nil
		},
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetect_Go_KafkaImport(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "producer"))
	mustWrite(t, filepath.Join(root, "producer", "main.go"),
		`package main

import (
	"github.com/IBM/sarama"
)

func main() { _ = sarama.NewConfig() }
`)

	result := runtime.Detect(context.Background(), root, noopRunner())

	if len(result.Signals) == 0 {
		t.Fatal("expected async signal for sarama import, got none")
	}
	found := false
	for _, s := range result.Signals {
		if s.Language == graph.LangGo && s.IntegrationKind == runtime.KindMessageQueue {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no go/message_queue signal; signals=%v", result.Signals)
	}
	if result.Confidence != "medium" {
		t.Errorf("confidence = %q, want %q", result.Confidence, "medium")
	}
}

const kafkaJSImport = `import { Kafka } from 'kafkajs';
const kafka = new Kafka({ brokers: ['localhost:9092'] });
`

const nestMessagePattern = `import { Controller } from '@nestjs/common';
import { MessagePattern } from '@nestjs/microservices';

@Controller()
export class AppController {
  @MessagePattern('order_created')
  handleOrder(data: any) {}
}
`

func TestDetect_TypeScript_KafkaJS(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{
			name:    "ts import",
			file:    "consumer.ts",
			content: kafkaJSImport,
		},
		{
			name:    "mts import",
			file:    "consumer.mts",
			content: kafkaJSImport,
		},
		{
			name:    "mjs import",
			file:    "consumer.mjs",
			content: kafkaJSImport,
		},
		{
			name:    "cts require",
			file:    "consumer.cts",
			content: "require('kafkajs')\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mustWrite(t, filepath.Join(root, tt.file), tt.content)

			result := runtime.Detect(context.Background(), root, noopRunner())

			if len(result.Signals) == 0 {
				t.Fatal("expected async signal for kafkajs import, got none")
			}
			found := false
			for _, s := range result.Signals {
				if s.Language == graph.LangTypeScript && s.Library == "kafkajs" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no typescript/kafkajs signal; signals=%v", result.Signals)
			}
		})
	}
}

func TestDetect_Python_Celery(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "tasks.py"),
		`from celery import Celery

app = Celery('tasks')

@app.task
def add(x, y):
    return x + y
`)

	result := runtime.Detect(context.Background(), root, noopRunner())

	if len(result.Signals) == 0 {
		t.Fatal("expected async signal for celery import, got none")
	}
	found := false
	for _, s := range result.Signals {
		if s.Language == graph.LangPython && s.Library == "celery" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no python/celery signal; signals=%v", result.Signals)
	}
}

func TestDetect_NestJS_MessagePattern(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{name: "ts decorator", file: "handler.ts"},
		{name: "mts decorator", file: "handler.mts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mustWrite(t, filepath.Join(root, tt.file), nestMessagePattern)

			result := runtime.Detect(context.Background(), root, noopRunner())

			found := false
			for _, s := range result.Signals {
				if s.Language == graph.LangTypeScript && s.Library == "@MessagePattern" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no @MessagePattern signal; signals=%v", result.Signals)
			}
		})
	}
}

func TestDetect_NoSignal_LowConfidence(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"),
		`package main

import "fmt"

func main() { fmt.Println("hello") }
`)

	result := runtime.Detect(context.Background(), root, noopRunner())

	if len(result.Signals) != 0 {
		t.Errorf("expected no signals, got %v", result.Signals)
	}
	if result.Confidence != "low" {
		t.Errorf("confidence = %q, want %q (absence of signal != synchronous)", result.Confidence, "low")
	}
}

func TestDetect_SkipsNodeModules(t *testing.T) {
	root := t.TempDir()
	nmDir := filepath.Join(root, "node_modules", "kafkajs")
	mustMkdir(t, nmDir)
	mustWrite(t, filepath.Join(nmDir, "index.ts"),
		`import { Kafka } from 'kafkajs';
`)

	result := runtime.Detect(context.Background(), root, noopRunner())

	for _, s := range result.Signals {
		if strings.HasPrefix(s.File, "node_modules") {
			t.Errorf("node_modules leaked: %v", s)
		}
	}
}

func TestDetect_Python_AsyncIO(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "service.py"),
		`import asyncio

async def main():
    await asyncio.sleep(1)
`)

	result := runtime.Detect(context.Background(), root, noopRunner())

	found := false
	for _, s := range result.Signals {
		if s.Language == graph.LangPython && s.IntegrationKind == runtime.KindAsyncTask {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no asyncio signal; signals=%v", result.Signals)
	}
}
