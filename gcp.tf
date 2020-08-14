variable "project" {}
variable "region" {}
variable "prefix" {}
variable "scheduler_cron" {}
variable "telegram_bot_token" {}
variable "telegram_chat_id" {}

provider "google" {
  project = var.project
  region  = var.region
}
resource "google_sourcerepo_repository" "repo" {
  name = "${var.prefix}-repository"
}

resource "google_pubsub_topic" "topic" {
  name = "${var.prefix}-trigger-topic"
}

resource "google_cloud_scheduler_job" "job" {
  name      = "${var.prefix}-scheduler"
  schedule  = var.scheduler_cron
  time_zone = "UTC"
  pubsub_target {
    topic_name = google_pubsub_topic.topic.id
    data       = base64encode("GO")
  }
}

resource "google_service_account" "function_account" {
  account_id = "${var.prefix}-function-runner"
}


resource "google_cloudfunctions_function" "function" {
  name                  = "${var.prefix}-function"
  runtime               = "go111"
  entry_point           = "Run"
  available_memory_mb   = 128
  timeout               = 60
  max_instances         = 1
  ingress_settings      = "ALLOW_INTERNAL_ONLY"
  service_account_email = google_service_account.function_account.email
  source_repository {
    url = "https://source.developers.google.com/projects/${var.project}/repos/${google_sourcerepo_repository.repo.name}/moveable-aliases/master"
  }

  event_trigger {
    event_type = "google.pubsub.topic.publish"
    resource   = google_pubsub_topic.topic.name
    failure_policy {
      retry = false
    }
  }
  environment_variables = {
    TELEGRAM_BOT_TOKEN = var.telegram_bot_token
    TELEGRAM_CHAT_ID   = var.telegram_chat_id
  }
}


resource "google_cloudfunctions_function_iam_member" "invoker" {
  project        = google_cloudfunctions_function.function.project
  region         = google_cloudfunctions_function.function.region
  cloud_function = google_cloudfunctions_function.function.name

  role   = "roles/cloudfunctions.invoker"
  member = "serviceAccount:${google_service_account.function_account.email}"
}
