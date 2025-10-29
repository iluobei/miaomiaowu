#!/bin/bash
# Test script to verify rule_templates are embedded in Docker image

set -e

echo "Building Docker image..."
docker build -t miaomiaowu-test:latest .

echo ""
echo "Testing if rule_templates directory is created on startup..."
docker run --rm -d --name miaomiaowu-test -p 8081:8080 miaomiaowu-test:latest

# Wait for container to start
sleep 5

echo ""
echo "Checking if rule_templates directory exists in container..."
docker exec miaomiaowu-test ls -la /app/rule_templates/

echo ""
echo "Listing rule template files..."
docker exec miaomiaowu-test find /app/rule_templates -name "*.yaml"

echo ""
echo "Cleaning up..."
docker stop miaomiaowu-test

echo ""
echo "Test completed successfully!"
