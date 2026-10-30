# nasdash

Единый TUI для NAS-Stack: торренты, рой агентов, задачи scheduler, музыка и метрики — в одном интерфейсе на bubbletea.

## Сборка и запуск

```sh
go build -o nasdash ./cmd/nasdash
./nasdash
```

Проверка всех сервисов без TUI:

```sh
./nasdash --selftest
```

## Вкладки

| # | Вкладка | Что делает |
|---|---|---|
| 1 | Торренты | `GET :8081/list`; `s` — поиск через `:8088`, `a` — добавить magnet, `d` — удалить, `p` — пауза/продолжить |
| 2 | Рой | `opencode run` тремя агентами (deepseek, glm, bunny) параллельно + синтез «Архитектора» |
| 3 | Задачи | `GET :8084/tasks`; `enter` — запуск через `POST /run/{name}` с `X-Admin-Token` |
| 4 | Музыка | `GET :8080/music/status` через gateway; `space`, `n`/`p`, `+`/`-` |
| 5 | Система | `GET :8082/all`; автообновление раз в 2 с |

## Клавиши

- `1`–`5` или `Tab`/`Shift+Tab` — вкладки
- `↑`/`↓` (`j`/`k`) — навигация, `r` — обновить
- `q` / `Ctrl+C` — выход
- На вкладке «Рой»: текст уходит в поле ввода, `Ctrl+Enter` — запуск, `Esc` — снять фокус

## Адреса

Переопределяются переменными окружения:

```
NASDASH_TORRENTS_URL=http://localhost:8081
NASDASH_STATS_URL=http://localhost:8082
NASDASH_SCHEDULER_URL=http://localhost:8084
NASDASH_MUSIC_URL=http://localhost:8080/music
NASDASH_SEARCH_URL=http://localhost:8088
```

`ADMIN_TOKEN` берётся из окружения или из `~/projects/nas-stack/.env` (в интерфейс не выводится).

## Особенности

- `music-service` не публикует порт 8092 наружу — доступ идёт через gateway (`:8080/music`), поэтому по умолчанию используется он.
- `scheduler` отдаёт только `name` и `schedule`; статусов выполнения в API нет, вкладка показывает «scheduled».
- Если indexers недоступны, `:8088/search` отвечает `502` — TUI показывает ошибку и не падает.
- Рой запускает `opencode run <prompt> --agent <agent> --model <model>` в отдельном временном каталоге; ответы панелей приходят по мере готовности.
