for i in $(seq -f "%03g" 1 10); do
  echo "Executing request with transaction ID: txn-$i"
  
  /home/nastya/go/bin/grpcurl -plaintext \
    -proto examples/naive_chain/pkg/chain/proto/consensus.proto \
    -d "{
      \"tx\": {
        \"client_id\": \"client\",
        \"id\": \"txn-$i\"
      }
    }" \
    localhost:7053 \
    consensus.TransactionService/SubmitTransaction
  
  echo "-----------------------------------------"
  # Small delay between requests to avoid overwhelming the server
  sleep 0.01
done