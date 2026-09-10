# Telegram News Monitor

A small production system for monitoring news across selected Telegram channels. It collects messages through MTProto, filters them by configurable keywords, stores matching items in PostgreSQL and uses a local Ollama model to classify relevant news before delivering it through Telegram. A separate search bot provides access to the recent news archive.

The project sits somewhere between a pet project and a small commercial solution. It started when a small business asked me to build a tool for their internal news monitoring workflow, and I took it on as a chance to solve a real problem while improving my own Go and backend engineering skills.

The system is currently running in production and is actively maintained by me. It is deployed as several Go services with Docker Compose and includes scheduled collection and delivery, PostgreSQL-backed processing, local LLM classification, fuzzy search and CI/CD through GitHub Actions.
