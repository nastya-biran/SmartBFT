#!/bin/bash

# Имя файла, в котором будет производиться замена
file="docker-compose.yml"

# Перебираем значения от 0 до 80 с шагом 10
for value in {0..80..10}; do
    # Заменяем текущее значение на новое
    sed -i "s/SPAM_MESSAGE_COUNT=[0-9]*/SPAM_MESSAGE_COUNT=$value/" "$file"
    
    echo "Значение изменено на: SPAM_MESSAGE_COUNT=$value"
    
    ./test.sh
    python3 experiments/parse_results.py $value
done

echo "Скрипт завершен. Финальное значение: SPAM_MESSAGE_COUNT=80"
./stop.sh
