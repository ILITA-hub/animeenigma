import time
import re
from requests.exceptions import SSLError
from gql import gql, Client
from gql.transport.requests import RequestsHTTPTransport

countPageAnime = 50
timeoutByBlockingIp = 30

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
                animes(limit: 50, page: %d, kind: "tv", score: 7) {
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
        """ % i)

        try:
            response = client.execute(query)
            animes = [*animes, *response.get("animes")]
            print(f"Прошла {i} страница. Осталось {countPageAnime - i} страниц")
            i += 1
        except SSLError as e:
            print(f"Прошла блокировка ip ждём {timeoutByBlockingIp} секунд")
            time.sleep(timeoutByBlockingIp)
            continue

    
    return animes

def filterAnime(animes):
    newAnimes = []
    for anime in animes:
        if len(anime.get("videos")) <= 0:
            continue
        
        newAnime = anime
        for op in anime['videos']:
            openings = []
            regexp = r"youtube.com"
            if op.get("kind") != "op" or op.get("kind") != "ed":
                continue

            if re.search(regexp, op.get("playerUrl")):
                openings.append(op)

            if len(newAnime.get("videos")) <= 0:
                continue

        newAnime['videos'] = openings
        newAnimes.append(newAnime)

    print(f"Было аниме: {len(animes)}, стало после фильтрации: {len(newAnimes)}")
    return newAnimes

def main():
    animes = getAnime()
    animes = filterAnime(animes)

if __name__ == "__main__":
    main()