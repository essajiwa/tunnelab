terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# ── Enable Compute API ────────────────────────────────────────────────────────

resource "google_project_service" "compute" {
  service            = "compute.googleapis.com"
  disable_on_destroy = false
}

# ── Static external IP ────────────────────────────────────────────────────────

resource "google_compute_address" "tunnelab" {
  name         = "tunnelab-ip"
  region       = var.region
  address_type = "EXTERNAL"
  depends_on   = [google_project_service.compute]
}

# ── Firewall: SSH ─────────────────────────────────────────────────────────────

resource "google_compute_firewall" "tunnelab_ssh" {
  name    = "tunnelab-allow-ssh"
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = var.ssh_allowed_ips
  target_tags   = ["tunnelab"]
  description   = "SSH access to TunneLab instance"

  depends_on = [google_project_service.compute]
}

# ── Firewall: Application ports ───────────────────────────────────────────────
#
# Port layout:
#   443         HTTPS proxy — Let's Encrypt TLS + public tunnel traffic
#   4443        WebSocket control channel — tunnel clients connect here
#   8080        HTTP proxy — plain HTTP tunnel traffic (or testing without TLS)
#   tcp_range   Configured TCP/UDP tunnel port range (default 50000-50010)
#   49152-65535 IANA ephemeral range — required for yamux mux back-connections
#
# Yamux note: the server calls net.Listen("tcp", ":0") per tunnel, getting a
# random OS-assigned port. It sends this port to the client over the WebSocket
# so the client can dial back. The firewall must allow this ephemeral range or
# tunnels will establish control but fail to carry data. Long-term fix: add a
# configurable fixed mux port to handler.go:waitForMuxConnection.

resource "google_compute_firewall" "tunnelab_app" {
  name    = "tunnelab-allow-app"
  network = "default"

  # Proxy and control ports
  allow {
    protocol = "tcp"
    ports    = ["80", "443", "4443"]
  }

  # TCP tunnel range
  allow {
    protocol = "tcp"
    ports    = [var.tcp_port_range]
  }

  # UDP tunnel range (same range, different L4 protocol)
  allow {
    protocol = "udp"
    ports    = [var.tcp_port_range]
  }

  # Ephemeral range for yamux mux back-connections from tunnel clients
  allow {
    protocol = "tcp"
    ports    = ["49152-65535"]
  }

  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["tunnelab"]
  description   = "TunneLab proxy, control, tunnel, and yamux mux traffic"

  depends_on = [google_project_service.compute]
}

# ── Dedicated service account ─────────────────────────────────────────────────

resource "google_service_account" "tunnelab" {
  account_id   = "tunnelab-sa"
  display_name = "TunneLab Server"
  depends_on   = [google_project_service.compute]
}

resource "google_project_iam_member" "tunnelab_logging" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.tunnelab.email}"
}

resource "google_project_iam_member" "tunnelab_monitoring" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.tunnelab.email}"
}

# ── Compute instance ──────────────────────────────────────────────────────────

resource "google_compute_instance" "tunnelab" {
  name         = "tunnelab-server"
  machine_type = var.machine_type
  zone         = var.zone
  tags         = ["tunnelab"]

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
      size  = var.disk_size_gb
      type  = "pd-balanced"
    }
  }

  network_interface {
    network = "default"
    access_config {
      nat_ip = google_compute_address.tunnelab.address
    }
  }

  metadata = {
    startup-script = templatefile("${path.module}/startup.sh.tpl", {
      domain         = var.domain
      tls_email      = var.tls_email
      tls_staging    = var.tls_staging
      repo_url       = var.repo_url
      tcp_port_range = var.tcp_port_range
    })
  }

  service_account {
    email = google_service_account.tunnelab.email
    scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring.write",
    ]
  }

  allow_stopping_for_update = true

  depends_on = [
    google_project_service.compute,
    google_compute_address.tunnelab,
    google_service_account.tunnelab,
  ]
}
