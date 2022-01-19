---
title: "Prometheus-мониторинг"
type:
  - instruction
search: prometheus
---

Устанавливает и полностью настраивает [Prometheus](https://prometheus.io/), настраивает сбор метрик со многих распространенных приложений, а также предоставляет необходимый минимальный набор alert'ов для Prometheus и dasboard для Grafana.

Устанавливается два экземпляра Prometheus:
* **main** — основной Prometheus, который выполняет scrape каждые 30 секунд (с помощью параметра `scrapeInterval` можно изменить это значение). Именно он обрабатывает все правила, отправляет алерты и является основным источником данных.
* **longterm** — дополнительный Prometheus, который выполняет scrape данных из main каждые 5 минут (с помощью параметра `longtermScrapeInterval` можно изменить это значение). Используется для продолжительного хранения истории и для отображения больших промежутков времени.

Если используется storage class с поддержкой автоматического расширения (`allowVolumeExpansion: true`), то при нехватке места на диске для данных Prometheus его ёмкость будет увеличена.

Ресурсы cpu и memory автоматически выставляются при пересоздании Pod'а на основе истории потребления, благодаря модулю [Vertical Pod Autoscaler](../../modules/302-vertical-pod-autoscaler/). Также, благодаря кешированию запросов к Prometheus с помощью trickster, потребление памяти Prometheus сильно сокращается.

Поддерживается как pull, так и push-модель получения метрик.

## Мониторинг аппаратных ресурсов
Реализовано отслеживание нагрузки на аппаратные ресурсы кластера с графиками по утилизации:
- процессора,
- памяти,
- диска,
- сети.

Графики доступны с агрегацией в разрезе по:
- Pod'ам,
- контроллерам,
- namespace'ам,
- узлам.

## Мониторинг Kubernetes

Deckhouse настраивает мониторинг широкого набора параметров “здоровья” Kubernetes и его компонентов, в частности:
- общей утилизации кластера;
- связанность узлов Kubernetes между собой (измеряется rtt между всеми узлами);
- доступность и работоспособность компонентов control-plane:
  - `etcd`
  - `coredns` и `kube-dns`
  - `kube-apiserver` и др.
- синхронизацию времени на узлах и др.

## Мониторинг Ingress

Подробно описан [здесь](../../modules/402-ingress-nginx/#мониторинг-и-статистика)

## Режим расширенного мониторинга
В Deckhouse возможно использование режима расширенного мониторинга, который дополнительно предоставляет возможности алертов по метрикам, собранным с помощью следующих экспортеров:
- `extended-monitoring-exporter`. Включает расширенный сбор метрик с namespace (у которых есть аннотация `extended-monitoring.flant.com/enabled=””`), в том числе сбор информации о доступных inodes и месте на дисках узлов, мониторинг утилизации узлов, доступность Pod'ов в Deployment, `StatefulSet`, `DaemonSet` и т.д.;
- `image-availability-exporter`.  Добавляет метрики и включает отправку алертов, позволяющих узнать о проблемах с доступностью образа контейнера в registry, прописанному в поле `image` из spec Pod’а в `Deployments`, `StatefulSets`, `DaemonSets`, `CronJobs`.

### Алертинг в режиме расширенного мониторинга
Deckhouse позволяет гибко настроить алертинг на каждый из namespace и указывать разную критичность в зависимости от порогового значения. Есть возможность указать множество пороговых значений отправки алертов в различных namespace, например, для таких параметров, как:
- значения свободного места и inodes на диске;
- утилизация CPU узлов и контейнера;
- процент 5xx ошибок на `nginx-ingress`;
- количество возможных недоступных Pod'ов в `Deployment`, `StatefulSet`, `DaemonSet`.

## Алерты
Мониторинг в составе Deckhouse включает также и возможности уведомления о событиях. В стандартной поставке уже идет большой набор только необходимых алертов, покрывающих состояние кластера и его компонентов. При этом всегда остается возможность добавления кастомных алертов.

### Отправка алертов во внешние системы
Deckhouse поддерживает отправку алертов с помощью `Alertmanager`:
- по протоколу SMTP;
- в PagerDuty;
- в Slack;
- в Telegram;
- посредством Webhook;
- и любым другим каналам, поддерживаемым в Alertmanager.