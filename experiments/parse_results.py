#!/usr/bin/env python3

import csv
import sys
from datetime import datetime

def parse_time(time_str):
    """Преобразует строку времени в datetime объект"""
    return datetime.strptime(time_str, "%H:%M:%S.%f")

def time_difference_in_milliseconds(time1, time2):
    """Вычисляет разницу между двумя временами в миллисекундах"""
    t1 = parse_time(time1)
    t2 = parse_time(time2)
    if t2 < t1:
        raise "Finish is before start"
    delta = t2 - t1
    # Преобразование разницы в миллисекунды (умножаем секунды на 1000)
    return delta.total_seconds()

def process_file_pair(file1_path, file2_path):
    """Обрабатывает пару файлов и возвращает списки t1 и (t2-t3)"""
    t1_values = []
    diff_values = []
    
    # Чтение файла 1 (содержит t1 и t2)
    file1_data = []
    with open(file1_path, 'r') as file1:
        for line in file1:
            parts = line.strip().split()
            if len(parts) >= 3:
                t1 = parts[-2]  # Предпоследнее значение
                t2 = parts[-1]  # Последнее значение
                file1_data.append((t1, t2))
            else:
                raise "Invalid file format"
    
    # Чтение файла 2 (содержит t3)
    file2_data = []
    with open(file2_path, 'r') as file2:
        for line in file2:
            t3 = line.strip()
            file2_data.append(t3)
    
    for i in range(min(len(file1_data), len(file2_data))):
        t1, t2 = file1_data[i]
        t3 = file2_data[i]
        
        try:
            # Вычисление разницы t2-t3 в миллисекундах
            t2_minus_t3_ms = time_difference_in_milliseconds(t3, t2)
            
            t1_values.append(float(t1))
            diff_values.append(t2_minus_t3_ms)
        except ValueError as e:
            print(f"Ошибка при обработке строки {i+1} в файлах {file1_path} и {file2_path}: {e}")
    
    return t1_values, diff_values

def main():
    
    if len(sys.argv) != 2:
        print("Использование: python вывод.csv")
        sys.exit(1)
    
    output_path = f"/home/nastbir/SmartBFT/experiments/results2/{sys.argv[1]}.csv"
    n = 3 

    results_internal = None
    results_end_to_end = None
    for i in range(1, n + 1):
        internal, end_to_end = process_file_pair(f"/home/nastbir/SmartBFT/metrics/{i}.txt", f"/home/nastbir/SmartBFT/experiments/start_time/{i}.txt")

        if results_internal is None:
            results_internal = internal
        else:
            results_internal = [x + y for x, y in zip(results_internal, internal)]

        if results_end_to_end is None:
            results_end_to_end = end_to_end
        else:
            results_end_to_end = [x + y for x, y in zip(results_end_to_end, end_to_end)]
    
    with open(output_path, 'w', newline='') as csvfile:
        csvwriter = csv.writer(csvfile)
        
        # Запись заголовка
        csvwriter.writerow(['internal', 'end-to-end'])

        for i in range(len(results_internal)):
            try:
                csvwriter.writerow([round(results_internal[i] / n, 3), round(results_end_to_end[i] / n, 3)])
            except ValueError as e:
                print(f"Ошибка при обработке строки {i+1}: {e}")
    
    print(f"Обработка завершена. Результаты сохранены в {output_path}")

if __name__ == "__main__":
    main()
