variable "project" {}
variable "region" {}
variable "prefix" {}
variable "scheduler_cron" {}
variable "telegram_bot_token" {}
variable "telegram_chat_id" {}
variable "rss_feed_url" {}

provider "google" {
  project = var.project
  region  = var.region
}
resource "google_sourcerepo_repository" "repo" {
  name = "${var.prefix}-repository"
  provisioner "local-exec" {
    command = "git remote add google ${google_sourcerepo_repository.repo.url} && git push google --all"
  }
  provisioner "local-exec" {
    when    = destroy
    command = "git remote remove google"
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
    url = "https://source.developers.google.com/projects/${var.project}/repos/${google_sourcerepo_repository.repo.name}/moveable-aliases/master/paths/rssbot/"
  }

  trigger_http = true
  environment_variables = {
    TELEGRAM_BOT_TOKEN = var.telegram_bot_token
    TELEGRAM_CHAT_ID   = var.telegram_chat_id
    RSS_FEED_URL       = var.rss_feed_url
  }
}


resource "google_cloudfunctions_function_iam_member" "invoker" {
  project        = google_cloudfunctions_function.function.project
  region         = google_cloudfunctions_function.function.region
  cloud_function = google_cloudfunctions_function.function.name

  role   = "roles/cloudfunctions.invoker"
  member = "serviceAccount:${google_service_account.function_account.email}"
}

resource "google_cloud_scheduler_job" "job" {
  name      = "${var.prefix}-scheduler"
  schedule  = var.scheduler_cron
  time_zone = "UTC"
  http_target {
    http_method = "GET"
    uri         = google_cloudfunctions_function.function.https_trigger_url

    oidc_token {
      service_account_email = google_service_account.function_account.email
    }
  }
}

