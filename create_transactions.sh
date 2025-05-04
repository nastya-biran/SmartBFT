for i in $(seq -f "%03g" 1 100); do
  echo "Executing request with transaction ID: txn-$i"
  
  # Определяем порт, начиная с 7051 и циклически используя до 7057
  # Используем остаток от деления (i-1) на 4 и прибавляем его к базовому порту
  for j in $(seq 0 3); do
    port=$((7051 + 2 * $j))
    
    echo "Using port: $port"
    
    grpcurl -plaintext \
      -proto examples/naive_chain/pkg/chain/proto/consensus.proto \
      -d "{
        \"tx\": {
          \"client_id\": \"client\",
          \"id\": \"txn-$i\"
        }
      }" \
      localhost:$port \
      consensus.TransactionService/SubmitTransaction
    
  done
  echo "-----------------------------------------"
  # Small delay between requests to avoid overwhelming the server
  sleep 1s
done

sleep 2m

docker ps -a | grep smartbft-node | awk '{print $1}' | xargs -r docker stop
docker ps -a | grep smartbft-node | awk '{print $1}' | xargs -r docker rm -f
