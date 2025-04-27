docker ps -a | grep smartbft_node | awk '{print $1}' | xargs -r docker stop
docker ps -a | grep smartbft_node | awk '{print $1}' | xargs -r docker rm -f