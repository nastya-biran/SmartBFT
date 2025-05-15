#!/bin/bash

for j in $(seq 0 3); do
  rm "experiments/start_time/$(($j + 1)).txt"
done

for i in $(seq -f "%03g" 1 5); do
  echo "Executing request with transaction ID: txn-$i"
  
  for j in $(seq 0 3); do
    port=$((7051 + 2 * $j))
    #port=$((7051 + ($(echo $i | sed 's/^0*//') - 1) % 3 * 2))
  
    echo "Using port: $port"
    
    current_time=$(date +%H:%M:%S.%3N)
    /home/nastbir/go/bin/grpcurl -plaintext \
      -proto examples/naive_chain/pkg/chain/proto/consensus.proto \
      -d "{
        \"tx\": {
          \"client_id\": \"client\",
          \"id\": \"txn-$i\"
        }
      }" \
      localhost:$port \
      consensus.TransactionService/SubmitTransaction
    echo "$current_time" >> "experiments/start_time/$((j + 1)).txt"
  done
  echo "-----------------------------------------"
  sleep 1s
done

sleep 10m

docker ps -a | grep smartbft-node | awk '{print $1}' | xargs -r docker stop
docker ps -a | grep smartbft-node | awk '{print $1}' | xargs -r docker rm -f
