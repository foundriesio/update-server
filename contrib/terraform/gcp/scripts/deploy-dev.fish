#!/usr/bin/env fish
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause-Clear

function usage
    printf '%s\n' \
        'Usage: contrib/terraform/gcp/scripts/deploy-dev.fish [--yes] [--build-only | --rollback]' \
        '' \
        'Builds a Linux/amd64 fioserver from this checkout and deploys it to' \
        'fioserver-server over IAP. A rollback binary is saved under /data.' \
        'Binary rollback does not reverse database migrations.' \
        'An interrupted run can leave a lock at /run/lock/fioserver-dev-deploy.lock.' \
        '' \
        '  --yes         Skip the live-deploy confirmation prompt' \
        '  --build-only  Build bin/fioserver-dev without connecting to GCP' \
        '  --rollback    Restore the previous binary saved on the data disk' \
        '  -h, --help    Show this help'
end

function release_deploy_lock --on-event fish_exit
    if set -q _fioserver_remote_lock
        if not gcloud compute ssh "$_fioserver_remote_instance" \
                --zone="$_fioserver_remote_zone" \
                --project="$_fioserver_remote_project" \
                --tunnel-through-iap \
                --command='sudo rmdir /run/lock/fioserver-dev-deploy.lock' >/dev/null 2>&1
            printf '%s\n' 'Could not release the VM deployment lock; check /run/lock/fioserver-dev-deploy.lock before retrying.' >&2
        end
    end
    if set -q _fioserver_deploy_lock
        rmdir "$_fioserver_deploy_lock" >/dev/null 2>&1
    end
end

function acquire_deploy_lock --argument-names lock_dir
    mkdir "$lock_dir" 2>/dev/null
    or begin
        printf 'Another deploy may be running, or a stale lock remains at %s.\n' "$lock_dir" >&2
        return 1
    end
    set -g _fioserver_deploy_lock "$lock_dir"
end

function remote_ssh --argument-names instance zone project remote_command
    gcloud compute ssh "$instance" \
        --zone="$zone" \
        --project="$project" \
        --tunnel-through-iap \
        --command="$remote_command"
end

function remote_scp --argument-names local_file instance zone project remote_file
    gcloud compute scp "$local_file" "$instance:$remote_file" \
        --zone="$zone" \
        --project="$project" \
        --tunnel-through-iap
end

function acquire_remote_lock --argument-names instance zone project
    remote_ssh "$instance" "$zone" "$project" 'sudo mkdir -m 0700 /run/lock/fioserver-dev-deploy.lock'
    or begin
        printf '%s\n' 'Another deploy may be active, or a stale VM lock exists at /run/lock/fioserver-dev-deploy.lock.' >&2
        return 1
    end
    set -g _fioserver_remote_lock yes
    set -g _fioserver_remote_instance "$instance"
    set -g _fioserver_remote_zone "$zone"
    set -g _fioserver_remote_project "$project"
end

function cleanup_remote_binary --argument-names instance zone project remote_binary
    remote_ssh "$instance" "$zone" "$project" "rm -f $remote_binary" >/dev/null 2>&1
end

function confirm_live --argument-names skip_prompt action
    if test "$skip_prompt" = yes
        return 0
    end

    if not status is-interactive
        printf 'Refusing to %s without an interactive confirmation; pass --yes.\n' "$action" >&2
        return 1
    end

    read --prompt-str="$action [y/N] " answer
    or return 1
    test "$answer" = y -o "$answer" = Y
end

function restore_binary --argument-names instance zone project source
    printf '%s\n' 'Restoring the previous server binary...'

    remote_ssh "$instance" "$zone" "$project" "sudo install -m 0755 $source /usr/local/bin/fioserver.rollback && sudo mv -f /usr/local/bin/fioserver.rollback /usr/local/bin/fioserver && sudo systemctl restart fioserver.service"
end

function check_instance_health --argument-names instance zone project
    for attempt in 1 2 3 4 5
        if remote_ssh "$instance" "$zone" "$project" 'sudo systemctl is-active --quiet fioserver.service && curl -fsS http://127.0.0.1:8080/healthz -o /dev/null'
            return 0
        end
        sleep 2
    end
    return 1
end

function check_installed_version --argument-names instance zone project expected
    set -l actual (remote_ssh "$instance" "$zone" "$project" '/usr/local/bin/fioserver --version')
    or return 1
    if test "$actual" != "$expected"
        printf 'Installed version mismatch: expected %s, got %s.\n' "$expected" "$actual" >&2
        return 1
    end
end

function check_health --argument-names health_url build_id
    for attempt in 1 2 3 4 5 6 7 8 9 10
        if curl --fail --silent --show-error --max-time 8 \
                --header 'Cache-Control: no-cache, no-store' \
                --header 'Pragma: no-cache' \
                "$health_url/healthz?deploy=$build_id" \
                --output /dev/null 2>/dev/null
            return 0
        end
        sleep 2
    end
    return 1
end

function rollback_attempt --argument-names instance zone project health_url
    restore_binary "$instance" "$zone" "$project" /data/.fioserver-dev/fioserver.attempt-previous
    or begin
        printf '%s\n' 'Automatic rollback failed; inspect fioserver.service on the VM.' >&2
        return 1
    end

    if check_instance_health "$instance" "$zone" "$project"; and check_health "$health_url" rollback
        printf '%s\n' 'Automatic rollback completed and health checks passed.' >&2
        return 0
    end

    printf '%s\n' 'Automatic rollback ran, but health checks still fail.' >&2
    return 1
end

function main
    argparse 'h/help' 'y/yes' 'b/build-only' 'r/rollback' -- $argv
    or return 2

    if set -q _flag_help
        usage
        return 0
    end

    if test (count $argv) -gt 0
        printf 'Unknown argument: %s\n' "$argv[1]" >&2
        usage >&2
        return 2
    end

    if set -q _flag_build_only; and set -q _flag_rollback
        printf '%s\n' '--build-only and --rollback cannot be combined.' >&2
        return 2
    end

    set -l instance fioserver-server
    set -l zone us-east1-b
    set -l project fioed-eric
    set -l health_url https://hearth.fioed-eric.io
    set -l skip_prompt no
    if set -q _flag_yes
        set skip_prompt yes
    end

    set -l repo_root (git rev-parse --show-toplevel 2>/dev/null)
    or begin
        printf '%s\n' 'Run this command from inside the update-server checkout.' >&2
        return 1
    end

    if not test -f "$repo_root/go.mod"
        printf '%s\n' 'The current Git root does not look like update-server.' >&2
        return 1
    end

    set -l cache_dir "$HOME/.cache/update-server"
    mkdir -p "$cache_dir"
    or return 1
    acquire_deploy_lock "$cache_dir/deploy.lock"
    or return 1

    if set -q _flag_rollback
        for tool in gcloud curl
            if not command -q "$tool"
                printf 'Required command not found: %s\n' "$tool" >&2
                return 1
            end
        end

        confirm_live "$skip_prompt" 'Restore the previous binary on fioserver-server'
        or return 1
        acquire_remote_lock "$instance" "$zone" "$project"
        or return 1

        remote_ssh "$instance" "$zone" "$project" 'sudo test -x /data/.fioserver-dev/fioserver.previous'
        or begin
            printf '%s\n' 'No saved rollback binary exists on the data disk.' >&2
            return 1
        end

        set -l current_version (remote_ssh "$instance" "$zone" "$project" '/usr/local/bin/fioserver --version')
        set -l previous_version (remote_ssh "$instance" "$zone" "$project" 'sudo /data/.fioserver-dev/fioserver.previous --version')
        if test -n "$current_version"; and test "$current_version" = "$previous_version"
            printf '%s\n' 'The saved rollback binary is already installed; refusing a no-op rollback.' >&2
            return 1
        end

        set -l save_current no
        if check_instance_health "$instance" "$zone" "$project"
            remote_ssh "$instance" "$zone" "$project" 'sudo cp -p /usr/local/bin/fioserver /data/.fioserver-dev/fioserver.attempt-previous'
            or return 1
            set save_current yes
        end

        restore_binary "$instance" "$zone" "$project" /data/.fioserver-dev/fioserver.previous
        or begin
            printf '%s\n' 'Rollback failed; inspect fioserver.service on the VM.' >&2
            return 1
        end

        if not check_instance_health "$instance" "$zone" "$project"; or not check_health "$health_url" rollback
            printf '%s\n' 'The previous binary was restored, but a health check failed.' >&2
            if test "$save_current" = yes
                restore_binary "$instance" "$zone" "$project" /data/.fioserver-dev/fioserver.attempt-previous
                or begin
                    printf '%s\n' 'Could not restore the pre-rollback binary; inspect fioserver.service on the VM.' >&2
                    return 1
                end
                if check_instance_health "$instance" "$zone" "$project"; and check_health "$health_url" rollback-recovery
                    remote_ssh "$instance" "$zone" "$project" 'sudo rm -f /data/.fioserver-dev/fioserver.attempt-previous' >/dev/null 2>&1
                    printf '%s\n' 'The pre-rollback binary was restored; the saved rollback binary remains on /data.' >&2
                else
                    printf '%s\n' 'The pre-rollback binary was restored, but health checks still fail.' >&2
                end
            else
                printf '%s\n' 'The saved rollback binary was retained on /data.' >&2
            end
            return 1
        end

        if test "$save_current" = yes
            remote_ssh "$instance" "$zone" "$project" 'sudo install -m 0755 /data/.fioserver-dev/fioserver.attempt-previous /data/.fioserver-dev/fioserver.previous.next && sudo mv -f /data/.fioserver-dev/fioserver.previous.next /data/.fioserver-dev/fioserver.previous'
            or return 1
            remote_ssh "$instance" "$zone" "$project" 'sudo rm -f /data/.fioserver-dev/fioserver.attempt-previous' >/dev/null 2>&1
        end

        printf '%s\n' 'Rollback completed and the UI health checks passed.'
        return 0
    end

    for tool in git docker
        if not command -q "$tool"
            printf 'Required command not found: %s\n' "$tool" >&2
            return 1
        end
    end

    set -l build_dir "$repo_root/bin"
    set -l go_build_cache "$cache_dir/go-build"
    set -l go_mod_cache "$cache_dir/go-mod"
    mkdir -p "$build_dir" "$go_build_cache" "$go_mod_cache"
    or return 1

    set -l revision (git -C "$repo_root" rev-parse --short HEAD)
    or return 1
    set -l build_id "$revision-dev-"(date -u +%Y%m%dT%H%M%SZ)
    set -l binary "$build_dir/fioserver-dev"
    set -l ldflags "-X=github.com/foundriesio/update-server/version.Version=$build_id"
    set -l uid (id -u)
    set -l gid (id -g)

    printf 'Building %s for Linux/amd64 from %s...\n' "$build_id" "$repo_root"
    docker run --rm --platform linux/amd64 \
        --user "$uid:$gid" \
        --env CGO_ENABLED=1 \
        --env HOME=/tmp \
        --env GOCACHE=/go-build-cache \
        --env GOMODCACHE=/go-mod-cache \
        --volume "$repo_root:/src" \
        --volume "$go_build_cache:/go-build-cache" \
        --volume "$go_mod_cache:/go-mod-cache" \
        --workdir /src \
        golang:1.27.3-bookworm \
        go build -trimpath -ldflags "$ldflags" -o /src/bin/fioserver-dev ./cmd/server
    or begin
        printf '%s\n' 'Linux build failed.' >&2
        return 1
    end

    if set -q _flag_build_only
        printf 'Build succeeded: %s\n' "$binary"
        return 0
    end

    if not command -q gcloud; or not command -q curl
        printf '%s\n' 'gcloud and curl are required to deploy.' >&2
        return 1
    end

    printf '\n%s\n' 'Working-tree changes included in this build:'
    git -C "$repo_root" status --short
    printf '\nDeploying %s to %s (%s, %s). This restarts the UI and device gateway.\n' \
        "$build_id" "$health_url" "$instance" "$zone"
    printf '%s\n' 'The server uses its existing database; binary rollback cannot undo database migrations.'
    confirm_live "$skip_prompt" 'Deploy this build to the live hearth server'
    or return 1
    acquire_remote_lock "$instance" "$zone" "$project"
    or return 1

    if not check_instance_health "$instance" "$zone" "$project"; or not check_health "$health_url" predeploy
        printf '%s\n' 'The existing server is unhealthy; refusing to deploy over it.' >&2
        return 1
    end

    set -l remote_binary "/tmp/fioserver-$build_id"
    remote_scp "$binary" "$instance" "$zone" "$project" "$remote_binary"
    or begin
        cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
        return 1
    end

    remote_ssh "$instance" "$zone" "$project" "chmod 0755 $remote_binary"
    or begin
        cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
        return 1
    end
    remote_ssh "$instance" "$zone" "$project" "$remote_binary --version"
    or begin
        cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
        return 1
    end

    remote_ssh "$instance" "$zone" "$project" 'sudo install -d -o root -g root -m 0700 /data/.fioserver-dev'
    or begin
        cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
        return 1
    end
    remote_ssh "$instance" "$zone" "$project" 'sudo cp -p /usr/local/bin/fioserver /data/.fioserver-dev/fioserver.attempt-previous'
    or begin
        cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
        return 1
    end
    remote_ssh "$instance" "$zone" "$project" "sudo install -m 0755 $remote_binary /usr/local/bin/fioserver.next && sudo mv -f /usr/local/bin/fioserver.next /usr/local/bin/fioserver && sudo systemctl restart fioserver.service"
    or begin
        rollback_attempt "$instance" "$zone" "$project" "$health_url"
        cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
        return 1
    end

    if not check_installed_version "$instance" "$zone" "$project" "$build_id"; or not check_instance_health "$instance" "$zone" "$project"; or not check_health "$health_url" "$build_id"
        printf '%s\n' 'Health check failed; rolling back the binary.' >&2
        rollback_attempt "$instance" "$zone" "$project" "$health_url"
        cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
        return 1
    end

    remote_ssh "$instance" "$zone" "$project" 'sudo install -m 0755 /data/.fioserver-dev/fioserver.attempt-previous /data/.fioserver-dev/fioserver.previous.next && sudo mv -f /data/.fioserver-dev/fioserver.previous.next /data/.fioserver-dev/fioserver.previous'
    or begin
        printf '%s\n' 'New build is healthy, but the rollback copy could not be promoted.' >&2
        return 1
    end
    remote_ssh "$instance" "$zone" "$project" 'sudo rm -f /data/.fioserver-dev/fioserver.attempt-previous' >/dev/null 2>&1

    cleanup_remote_binary "$instance" "$zone" "$project" "$remote_binary"
    printf 'Deployed %s; instance and public UI health checks passed. Previous healthy binary is saved on /data.\n' "$build_id"
end

main $argv
