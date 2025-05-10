import os
import pandas as pd
import glob
import re
from pathlib import Path

def merge_csv_files():
    # Получаем текущую директорию
    current_dir = os.path.abspath("/home/nastbir/SmartBFT/experiments")
    
    # Определяем префикс директорий, которые нужно искать
    prefix = "fixed"
    
    # Находим все директории, начинающиеся с prefix
    result_dirs = [d for d in os.listdir(current_dir) if os.path.isdir(os.path.join(current_dir, d)) and d.startswith(prefix)]
    
    if not result_dirs:
        print(f"Директории с префиксом '{prefix}' не найдены.")
        return
    
    print(f"Найдены директории: {', '.join(result_dirs)}")
    
    # Создаем словарь для хранения путей к CSV файлам с одинаковыми названиями
    csv_files_map = {}
    
    # Обходим все директории с префиксом results
    for dir_name in result_dirs:
        dir_path = os.path.join(current_dir, dir_name)
        
        # Ищем все CSV файлы в директории
        for csv_file in glob.glob(os.path.join(dir_path, "*.csv")):
            # Получаем только имя файла без пути
            file_name = os.path.basename(csv_file)
            
            # Добавляем путь к файлу в словарь под ключом имени файла
            if file_name not in csv_files_map:
                csv_files_map[file_name] = []
            
            csv_files_map[file_name].append(csv_file)
    
    # Создаем директорию для объединенных файлов, если она не существует
    merged_dir = os.path.join(current_dir, "merged_fixed_csv")
    os.makedirs(merged_dir, exist_ok=True)
    
    # Обрабатываем каждое имя файла
    for file_name, file_paths in csv_files_map.items():
        if len(file_paths) > 1:
            print(f"Объединение {len(file_paths)} файлов с именем {file_name}...")
            
            # Список для хранения всех dataframes
            dfs = []
            
            # Загружаем все CSV файлы
            for file_path in file_paths:
                try:
                    df = pd.read_csv(file_path)
                    
                    # Добавляем столбец с источником данных (именем директории)
                    dir_name = os.path.basename(os.path.dirname(file_path))
                    df['source_directory'] = dir_name
                    
                    dfs.append(df)
                except Exception as e:
                    print(f"Ошибка при чтении файла {file_path}: {e}")
            
            if dfs:
                # Объединяем все dataframes
                merged_df = pd.concat(dfs, ignore_index=True)
                
                # Сохраняем объединенный файл
                output_path = os.path.join(merged_dir, file_name.replace(".csv", "_fixed.csv"))
                merged_df.to_csv(output_path, index=False)
                print(f"Сохранен объединенный файл: {output_path}")
            else:
                print(f"Не удалось объединить файлы для {file_name}. Пропускаем.")
        else:
            print(f"Найден только один файл с именем {file_name}. Копируем его без изменений.")
            # Копируем единственный файл в директорию для объединенных файлов
            try:
                df = pd.read_csv(file_paths[0])
                # Добавляем столбец с источником данных
                dir_name = os.path.basename(os.path.dirname(file_paths[0]))
                df['source_directory'] = dir_name
                
                output_path = os.path.join(merged_dir, file_name.replace(".csv", "_fixed.csv"))
                df.to_csv(output_path, index=False)
                print(f"Скопирован файл: {output_path}")
            except Exception as e:
                print(f"Ошибка при копировании файла {file_paths[0]}: {e}")
    
    print("\nОбработка завершена.")
    if csv_files_map:
        print(f"Все объединенные файлы сохранены в директории: {merged_dir}")
    else:
        print("CSV файлы не найдены в указанных директориях.")

if __name__ == "__main__":
    merge_csv_files()
