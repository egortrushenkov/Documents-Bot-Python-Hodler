import json
import os

CLIENTS_PATH = os.path.join(os.path.dirname(__file__), "..", "data", "clients.json")


def load() -> dict:
    if os.path.exists(CLIENTS_PATH):
        with open(CLIENTS_PATH, "r", encoding="utf-8") as f:
            return json.load(f)
    return {}


def save(clients: dict):
    os.makedirs(os.path.dirname(CLIENTS_PATH), exist_ok=True)
    with open(CLIENTS_PATH, "w", encoding="utf-8") as f:
        json.dump(clients, f, ensure_ascii=False, indent=2)


def add(client: dict):
    clients = load()
    clients[client["inn"]] = client
    save(clients)


def list_clients() -> list[dict]:
    return list(load().values())


def get_by_inn(inn: str) -> dict | None:
    return load().get(inn)
