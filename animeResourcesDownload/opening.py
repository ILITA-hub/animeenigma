import time
import re
import yt_dlp
import boto3
import os
import threading
import psycopg2
from dotenv import load_dotenv
from botocore.client import Config
from requests.exceptions import SSLError
from gql import gql, Client
from gql.transport.requests import RequestsHTTPTransport

load_dotenv()

access_key = os.getenv("ACCESS_KEY")
secret_key = os.getenv("SECRET_KEY")
countPageAnime = 3
countAnime = 50
timeoutByBlockingIp = 30
countThread = 3

session = boto3.session.Session()
s3_client = session.client(
    service_name = 's3', 
    endpoint_url = 'https://hb.bizmrg.com',
    aws_access_key_id=access_key,
    aws_secret_access_key=secret_key,
    config=Config(signature_version='s3v4')
)
test_bucket_name = 'animeenigmaopenings'

# URL вашего GraphQL сервера
url = 'https://shikimori.one/api/graphql'

transport = RequestsHTTPTransport(
    url=url,
    use_json=True,
    headers={
        "User-Agent": "Mozilla/5.0",
        "Accept": "application/json",
        "Content-Type": "application/json"
    }
)

# Создаем клиент
client = Client(transport=transport, fetch_schema_from_transport=True)

def getAnime():
    animes = []
    i = 1
    while i <= countPageAnime :
        query = gql("""
            query Animes {
                animes(limit: %d, page: %d, kind: "tv", score: 7) {
                    name
                    english
                    id
                    japanese
                    russian
                    videos {
                        id
                        playerUrl
                        name
                        kind
                    }
                    genres {
                        id
                        name
                        russian
                    }
                    airedOn {
                        year
                    }
                    poster {
                        originalUrl
                    }
                }
            }  
        """ % (countAnime, i))

        try:
            response = client.execute(query)
            animes = [*animes, *response.get("animes")]
            print(f"Прошла {i} страница. Осталось {countPageAnime - i} страниц")
            i += 1
        except SSLError as e:
            print(f"Произошла блокировка ip ждём {timeoutByBlockingIp} секунд")
            time.sleep(timeoutByBlockingIp)
            continue

    
    return animes

def filterAnime(animes):
    newAnimes = []
    for anime in animes:
        if len(anime.get("videos")) <= 0:
            continue
        
        newAnime = anime
        regexp = r"youtube.com"
        for op in anime['videos']:
            openings = []

            if op.get("kind") == "op" or op.get("kind") == "ed":
                if re.search(regexp, op.get("playerUrl")):
                    openings.append(op)

        newAnime['videos'] = openings

        if len(newAnime.get("videos")) <= 0:
            continue

        newAnimes.append(newAnime)

    print(f"Было аниме: {len(animes)}, стало после фильтрации: {len(newAnimes)}")
    return newAnimes

def install_openings_and_add_cloud(animes, thred, connection):
    countDownloadOpenings = 0
    cursor = connection.cursor()
    print(f"Поток {thred}, начал скачку аниме")
    for anime in animes:
        add_anime(anime, cursor, thred)
        connection.commit()
        for video in anime.get('videos'):
            outName = f"{video.get("id")}.mp4"
            ydl_opts = {
                'format': 'best',  # скачиваем видео в лучшем доступном качестве
                'outtmpl': outName,  # название файла будет совпадать с названием видео
            }
            with yt_dlp.YoutubeDL(ydl_opts) as ydl:
                try:
                    ydl.download([video.get("playerUrl")])
                    s3_client.upload_file(outName, test_bucket_name, outName, ExtraArgs={'ACL': 'public-read'})
                    countDownloadOpenings += 1
                    os.remove(outName)
                    id = video.get("id")
                    mp4Path = video.get("playerUrl")
                    name = video.get("name", anime.get("name", "Название аниме"))
                    animeId = anime.get("id")
                    kind = video.get("kind")
                    insert_query = """INSERT INTO public.videos  (id, "mp4Path", "name", "animeId", "active", "kind") VALUES (%s, %s, %s, %s, true, %s)"""
                    record_to_insert = (id, mp4Path, name, animeId, kind)
                    cursor.execute(insert_query, record_to_insert)
                except (Exception, psycopg2.Error) as error:
                    print(f"Поток {thred} Ошибка при работе с опенингами: {error}")

        connection.commit()

    print(f"Поток {thred}, закончил скачку опенингов")
    cursor.close()

def split_array(array, n):
    if n <= 0:
        raise ValueError("n должно быть положительным числом")
    
    # Если n больше длины массива, вернуть исходный массив без изменений
    if n > len(array):
        return [array]

    k, m = divmod(len(array), n)
    return [array[i * k + min(i, m):(i + 1) * k + min(i + 1, m)] for i in range(n)]

def add_anime(anime, cursor, thred):
    try:
        id = anime.get("id")
        name = anime.get("name", "Название аниме")
        nameRu = anime.get("russian", name)
        nameJP = anime.get("japanese", name)
        year = anime.get("airedOn").get("year")
        imgPath = anime.get("poster").get("originalUrl")

        insert_query = """INSERT INTO public.anime (id, "name", "nameRU", "nameJP", "active", "year", "imgPath") VALUES (%s, %s, %s, %s, true, %s, %s)"""
        record_to_insert = (id, name, nameRu, nameJP, year, imgPath)
        cursor.execute(insert_query, record_to_insert)

        for genre in anime.get("genres"):
            insert_query = """INSERT INTO public."genresAnime" ("animeId", "genreId", "active") VALUES (%s, %s, true)"""
            record_to_insert = (id, genre.get("id"))
            cursor.execute(insert_query, record_to_insert)

        print(f"Поток {thred} Добавилось аниме с id - {id} и name - {nameRu} ")
    except (Exception, psycopg2.Error) as error:
        print("Поток {thred} Ошибка при работе с PostgreSQL:", error)

def main():
    animes = getAnime()
    animes = filterAnime(animes)
    threads = []
    animes = split_array(animes, countThread)
    connection = psycopg2.connect(
        user="postgresUserAE",
        password="pgSuperSecretMnogaBycaBab",
        host="localhost",  # или адрес сервера
        port="15432",
        database="animeenigma"
    )
    for i in range(countThread):
        thread = threading.Thread(target=install_openings_and_add_cloud, args=(animes[i], i, connection))
        threads.append(thread)
        thread.start()

    for thread in threads:
        thread.join()

    connection.close()

if __name__ == "__main__":
    main()