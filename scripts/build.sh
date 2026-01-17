#!/bin/bash
set -e

echo "🔨 Building QuantumFlow..."

# Build the binary
go build -o bin/quantumflow -ldflags="-w -s" cmd/quantumflow/main.go

# Check binary size
if [ -f "bin/quantumflow" ]; then
    SIZE=$(du -h bin/quantumflow | cut -f1)
    echo "✓ Build successful! Binary size: $SIZE"
    echo "Run with: ./bin/quantumflow"
else
    echo "❌ Build failed"
    exit 1
fi
