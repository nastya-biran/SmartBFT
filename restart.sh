docker ps -a | grep smartbft-node | awk '{print $1}' | xargs -r docker stop
docker compose up -d