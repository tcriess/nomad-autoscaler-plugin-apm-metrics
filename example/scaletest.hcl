job "scaletest" {
  datacenters = [
    "eu-west-1"]
  region = "eu"
  type = "service"
  constraint {
    attribute = "${attr.unique.consul.name}"
    value = "monitor-eu"
  }
  group "scaletest" {

    scaling {
      enabled = true
      min = 1
      max = 2
      policy {
        check "scaletest" {
          source = "drone-metrics"
          // query = "drone_pending_jobs"
          query = "go_threads"

          strategy "threshold" {
            upper_bound = 20
            lower_bound = 2
            delta = 1
          }
        }
      }
    }

    network {
      port "redis" {
        to = 6379
      }
    }

    task "scaletest" {
      /*
      driver = "docker"
      config {
        image = "redis:7"
        volumes = []
        ports = ["redis"]
        // port_map {
        //  redis = 6379
        // }
      }
      */
      driver       = "ecs"
      kill_timeout = "2m" // increased from default to accomodate ECS.

      config {
        port_map {
          redis = 6379
        }
        advertise = true
        inline_task_definition = true
        task {
          tags {
            key = "number"
            value = "1-2"
          }
          launch_type     = "FARGATE"
          task_definition = "scaletest" // only used if inline_task_definition = false
          network_configuration {
            aws_vpc_configuration {
              // assign_public_ip = "ENABLED"
              security_groups  = ["sg-028b54cb9e426ad86"] // ecs-keycloak-securitygroup, just for testing
              subnets          = ["subnet-0ebb96e29da1782b7"] // subnet-private-aza
            }
          }
        }
        // access key AKIA5DABMKOFIWQYA4TK
        // secret key 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
        task_definition {
            family = "scaletest"
            container_definitions {
              name = "scaletest"
              image = "redis:7"
              cpu = 512
              memory = 1024
              port_mappings {
                container_port = 6379
                host_port      = 6379
                protocol       = "tcp"
              }
            }
            cpu = "512"
            memory = "1024"
            execution_role_arn = "arn:aws:iam::899798553482:role/nomad-role-eu"
        }
      }
      env {}
      resources {
        cpu = 500
        memory = 1024
      }
      service {
        name = "scaletest"
        tags = ["monitor:scaletest"]
        port = "redis"
        check {
          type     = "tcp"
          port     = "redis"
          interval = "30s"
          timeout  = "5s"
        }
      }
    }
  }
}