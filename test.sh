#!/bin/bash

set -e

echo "Stopping and removing all smartbft containers..."
docker ps -a | grep smartbft-node | awk '{print $1}' | xargs -r docker stop
docker ps -a | grep smartbft-node | awk '{print $1}' | xargs -r docker rm -f

# Функция для проверки состояния контейнера
check_container() {
    local container_name=$1
    local status=$(docker inspect -f '{{.State.Status}}' $container_name 2>/dev/null || echo "not_found")
    
    if [ "$status" != "running" ]; then
        echo "Error: Container $container_name is not running (status: $status)"
        docker logs $container_name
        exit 1
    fi
}

echo "Cleaning up previous data..."
docker compose down --volumes --remove-orphans
rm -rf data/node*
rm -rf metrics/*

echo "Creating data directories..."
mkdir -p data/node{1,2,3,4}
chmod -R 777 data/

echo "Building Docker images..."
docker compose build node1

echo "Starting the network..."
docker compose up -d

echo "Waiting for initialization..."
sleep 5

# Проверяем состояние каждой ноды
for i in {1..4}; do
    container_name="smartbft-node${i}-1"
    check_container $container_name
    echo "Checked node ${i}"
done

echo "All nodes are running. Use 'docker-compose logs -f' to see the logs.\n\n\n" 

echo "All transactions submitted successfully!"