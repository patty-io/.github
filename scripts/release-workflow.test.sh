#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow="$repo_root/.github/workflows/container-release.yml"
template="$repo_root/workflow-templates/patty-container-release.yml"
promotion_task="$repo_root/kargo/release-metadata-task.yaml"
promote_app_task="$repo_root/kargo/promote-app-task.yaml"

actionlint "$workflow" "$template"

ruby -e '
  require "yaml"

  workflow, template, promotion_task, promote_app_task = ARGV
  reusable = YAML.safe_load_file(workflow, aliases: true)
  caller = YAML.safe_load_file(template, aliases: true)
  task = YAML.safe_load_file(promotion_task, aliases: true)
  promote = YAML.safe_load_file(promote_app_task, aliases: true)

  trigger = reusable.fetch(true).fetch("workflow_call")
  inputs = trigger.fetch("inputs")
  raise "missing images input" unless inputs.dig("images", "required") == true
  raise "missing registry username secret" unless trigger.dig("secrets", "registry_username", "required") == true
  raise "missing registry password secret" unless trigger.dig("secrets", "registry_password", "required") == true

  jobs = reusable.fetch("jobs")
  source = jobs.fetch("source")
  build = jobs.fetch("build")
  raise "source metadata must be resolved once" unless build.fetch("needs") == "source"
  raise "workflow must use the caller image matrix" unless build.dig("strategy", "matrix", "include") == "${{ fromJSON(inputs.images) }}"
  tag = build.fetch("steps").find { |step| step["id"] == "metadata" }.dig("with", "tags")
  raise "release tags must be unique across reruns" unless tag.include?("github.run_id") && tag.include?("github.run_attempt")
  steps = build.fetch("steps")
  metadata = steps.find { |step| step["id"] == "metadata" }
  raise "missing docker metadata step" unless metadata&.fetch("uses", "").start_with?("docker/metadata-action@")
  levels = metadata.dig("env", "DOCKER_METADATA_ANNOTATIONS_LEVELS")
  raise "annotations must cover manifest and index" unless levels == "manifest,index"
  annotations = metadata.dig("with", "annotations")
  %w[
    org.opencontainers.image.source
    org.opencontainers.image.revision
    org.opencontainers.image.created
    org.opencontainers.image.version
    org.opencontainers.image.title
    org.opencontainers.image.description
    io.patty.ci.workflow-url
    io.patty.ci.result
    io.patty.git.commit-url
    io.patty.git.pull-requests
    io.patty.git.changed-files
  ].each do |key|
    raise "missing #{key}" unless annotations.include?(key + "=")
  end
  push = steps.find { |step| step.fetch("uses", "").start_with?("docker/build-push-action@") }
  raise "build must publish generated annotations" unless push&.dig("with", "annotations") == "${{ steps.metadata.outputs.annotations }}"

  template_jobs = caller.fetch("jobs")
  release = template_jobs.fetch("release")
  raise "template must pin the shared workflow" unless release.fetch("uses").match?(%r{\Apatty-io/\.github/\.github/workflows/container-release\.yml@[0-9a-f]{40}\z})

  raise "promotion metadata manifest must contain two tasks" unless task.fetch("kind") == "ClusterPromotionTask"

  raise "promote-app task name" unless promote.dig("metadata", "name") == "patty-promote-app"
  promote_vars = promote.fetch("spec").fetch("vars").map { |v| v.fetch("name") }
  %w[repoURL branch overlayPath commitMessage].each do |name|
    raise "promote-app missing var #{name}" unless promote_vars.include?(name)
  end
  promote_steps = promote.fetch("spec").fetch("steps").map { |s| s.fetch("uses") }
  unless promote_steps == %w[git-clone kustomize-set-image git-commit git-push]
    raise "promote-app must clone, pin, commit, push (got: #{promote_steps.inspect})"
  end
' "$workflow" "$template" "$promotion_task" "$promote_app_task"
