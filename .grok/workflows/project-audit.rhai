// Project health audit: docs drift, non-functional/dead code, security.
// Default: report only. Pass args.apply_docs=true to fix confirmed doc issues after synthesis.
let meta = #{
    name: "project-audit",
    description: "Audit project state: outdated docs, non-functional code, and security issues",
    when_to_use: "When you want a structured health check of Launchpad docs, dead code, and security posture",
    phases: [
        #{ title: "Audit", detail: "parallel scanners: docs, dead code, security" },
        #{ title: "Verify", detail: "adversarial check of each finding" },
        #{ title: "Report", detail: "synthesize markdown report" },
        #{ title: "Docs", detail: "optional confirmed doc fixes" },
    ],
};

let findings_schema = #{
    "type": "object",
    "required": ["findings"],
    "properties": #{
        "findings": #{
            "type": "array",
            "maxItems": 8,
            "items": #{
                "type": "object",
                "required": ["category", "severity", "file", "issue", "evidence"],
                "properties": #{
                    "category": #{ "type": "string", "enum": ["docs", "dead_code", "security", "state"] },
                    "severity": #{ "type": "string", "enum": ["critical", "high", "medium", "low"] },
                    "file": #{ "type": "string" },
                    "issue": #{ "type": "string" },
                    "evidence": #{ "type": "string" },
                    "suggested_fix": #{ "type": "string" },
                },
            },
        },
        "notes": #{ "type": "string" },
    },
};

let verdict_schema = #{
    "type": "object",
    "required": ["real", "reason", "evidence"],
    "properties": #{
        "real": #{ "type": "boolean" },
        "reason": #{ "type": "string" },
        "evidence": #{ "type": "string" },
        "severity_adjust": #{ "type": "string" },
    },
};

let report_schema = #{
    "type": "object",
    "required": ["markdown", "summary"],
    "properties": #{
        "markdown": #{ "type": "string" },
        "summary": #{ "type": "string" },
        "priority_actions": #{
            "type": "array",
            "maxItems": 10,
            "items": #{ "type": "string" },
        },
    },
};

let apply_docs = false;
if args != () && args.apply_docs != () {
    apply_docs = args.apply_docs == true || args.apply_docs == "true";
}

// ---- Phase: Audit (parallel dimensions) ----
phase("Audit");
log("Starting parallel audit: docs, dead_code, security, project_state");

let audit_jobs = [];

// Docs drift scanner
let docs_prompt = "";
docs_prompt += "You are auditing the Launchpad Go monorepo for OUTDATED DOCUMENTATION.\n";
docs_prompt += "Repo root is the current workspace. Use read-only tools only: read_file, list_dir, grep, run_terminal_command for git/ls only.\n";
docs_prompt += "Do NOT answer from memory — open the actual files.\n\n";
docs_prompt += "MUST inspect (at minimum):\n";
docs_prompt += "1. README.md vs real CLI (cmd/launchpad, internal/cli) and make targets (Makefile)\n";
docs_prompt += "2. AGENTS.md vs current layout, MVP scope, e2e commands, deferred items\n";
docs_prompt += "3. docs/DOMAIN.md vs internal/domain + internal/api handlers + store migrations\n";
docs_prompt += "4. docs/DESIGN.md vs actual architecture (jobs, auth, targets)\n";
docs_prompt += "5. docs/openapi.yaml vs registered routes in internal/api (handlers, router)\n";
docs_prompt += "6. docs/superpowers/program/QUEUE.md freshness vs what is already implemented\n";
docs_prompt += "7. Skill files under .grok/skills/ if they contradict AGENTS.md or README\n\n";
docs_prompt += "Look for: wrong paths, removed APIs still documented, missing shipped features, ";
docs_prompt += "App model leftovers, wrong default env, outdated bootstrap flows, OpenAPI drift.\n";
docs_prompt += "Report at most 8 concrete findings. category MUST be \"docs\".\n";
docs_prompt += "Each finding: file path, issue, evidence (quote or specific mismatch), suggested_fix.\n";
docs_prompt += "Empty findings is valid ONLY after you have read the key docs and compared to code.\n";

audit_jobs.push(#{
    prompt: docs_prompt,
    label: "audit:docs",
    capability_mode: "read-only",
    output_schema: findings_schema,
});

// Dead / non-functional code scanner
let dead_prompt = "";
dead_prompt += "You are auditing the Launchpad Go monorepo for NON-FUNCTIONAL or DEAD CODE.\n";
dead_prompt += "Use read-only tools: read_file, list_dir, grep, and shell only for go list / git if needed via run_terminal_command.\n";
dead_prompt += "Do NOT answer from memory — inspect the code.\n\n";
dead_prompt += "Search for:\n";
dead_prompt += "1. Unreachable stubs: panic(\"TODO\"), panic(\"not implemented\"), return ErrNotImplemented permanently\n";
dead_prompt += "2. Dead code: unexported functions never called, files never imported, build tags that exclude real paths\n";
dead_prompt += "3. Broken wiring: handlers registered but not implemented, CLI commands that always error, empty switch cases\n";
dead_prompt += "4. Leftover App model or /v1/apps remnants\n";
dead_prompt += "5. Interfaces with only stub implementations that claim production readiness\n";
dead_prompt += "6. Tests that skip everything or tests that cannot pass (t.Skip without reason for permanent disable)\n";
dead_prompt += "7. Makefile targets that reference missing scripts\n\n";
dead_prompt += "Prefer high-confidence findings. Do not flag intentional deferred features that are clearly documented as deferred.\n";
dead_prompt += "Report at most 8 findings. category MUST be \"dead_code\".\n";
dead_prompt += "Each: file, issue, evidence (symbol names, call-site absence), suggested_fix.\n";
dead_prompt += "Empty list valid ONLY after grepping for TODO/FIXME/panic and scanning main packages.\n";

audit_jobs.push(#{
    prompt: dead_prompt,
    label: "audit:dead_code",
    capability_mode: "read-only",
    output_schema: findings_schema,
});

// Security scanner
let sec_prompt = "";
sec_prompt += "You are auditing the Launchpad Go monorepo for SECURITY ISSUES.\n";
sec_prompt += "Use read-only tools: read_file, list_dir, grep. Inspect actual code — do not invent issues.\n\n";
sec_prompt += "Focus areas (MVP control plane):\n";
sec_prompt += "1. Auth: token validation, bootstrap token handling, workspace scoping bypasses (internal/auth, API middleware)\n";
sec_prompt += "2. Secrets: encryption at rest, redaction in API responses/logs, secret typing (internal/secrets, config)\n";
sec_prompt += "3. Injection: SQL placeholder misuse, command injection in CLI/target, path traversal\n";
sec_prompt += "4. Sensitive data in logs, error messages, or problem+json details\n";
sec_prompt += "5. Kubernetes target: RBAC assumptions, secret mounting, resource naming collisions\n";
sec_prompt += "6. Hardcoded credentials, weak defaults, insecure TLS skip in production paths\n";
sec_prompt += "7. IDOR-style access: can token A act on workspace B resources?\n";
sec_prompt += "8. .gitignore gaps for .env, *.db with secrets, kubeconfigs committed\n\n";
sec_prompt += "Severity guide: critical=exploitable auth/secrets bypass; high=likely leak or privilege issue; medium=defense-in-depth; low=hygiene.\n";
sec_prompt += "Report at most 8 findings. category MUST be \"security\".\n";
sec_prompt += "Each: file, issue, evidence, suggested_fix. Prefer concrete code evidence over generic advice.\n";
sec_prompt += "Empty list valid ONLY after inspecting auth, secrets, store SQL, and API response paths.\n";

audit_jobs.push(#{
    prompt: sec_prompt,
    label: "audit:security",
    capability_mode: "read-only",
    output_schema: findings_schema,
});

// Project state / consistency scanner
let state_prompt = "";
state_prompt += "You are auditing overall PROJECT STATE consistency for Launchpad.\n";
state_prompt += "Use read-only tools. Inspect the repo — do not answer from memory.\n\n";
state_prompt += "Check:\n";
state_prompt += "1. go.mod / mise.toml toolchain alignment with AGENTS.md (Go version)\n";
state_prompt += "2. CI workflows (.github/workflows) vs make targets and documented e2e paths\n";
state_prompt += "3. Migration numbering continuity (internal/store/migrations) postgres+sqlite pairs\n";
state_prompt += "4. Specs/plans that claim \"done\" vs code; or shipped code with no docs\n";
state_prompt += "5. Branch/worktree hygiene notes if .worktrees or leftover artifacts are tracked\n";
state_prompt += "6. Whether examples/hello-stub still matches current CLI\n\n";
state_prompt += "Report at most 6 findings. category MUST be \"state\".\n";
state_prompt += "Each: file, issue, evidence, suggested_fix. High-confidence only.\n";

audit_jobs.push(#{
    prompt: state_prompt,
    label: "audit:state",
    capability_mode: "read-only",
    output_schema: findings_schema,
});

let audit_results = parallel(audit_jobs);

let findings = [];
let audit_i = 0;
for r in audit_results {
    if r == () || !r.success {
        log("audit panel slot " + audit_i.to_string() + " failed or empty — treating as no findings from that scanner");
    } else if r.output.findings != () {
        for f in r.output.findings {
            // Normalize required fields with guards
            if f != () && f.file != () && f.issue != () {
                let cat = if f.category != () { f.category } else { "state" };
                let sev = if f.severity != () { f.severity } else { "medium" };
                let ev = if f.evidence != () { f.evidence } else { "" };
                let fix = if f.suggested_fix != () { f.suggested_fix } else { "" };
                findings.push(#{
                    category: cat,
                    severity: sev,
                    file: f.file,
                    issue: f.issue,
                    evidence: ev,
                    suggested_fix: fix,
                });
            }
        }
    }
    audit_i += 1;
}

log("Collected " + findings.len().to_string() + " raw findings before verification");

if findings.len() == 0 {
    let empty_md = "# Project audit report\n\nNo findings from scanners (or all scanners failed).\n\n";
    empty_md += "Scanners: docs, dead_code, security, project_state.\n";
    empty_md += "If this seems wrong, re-run with a higher agent budget or inspect `/workflows` agent logs.\n";
    let p = write_scratch_file("report.md", empty_md);
    complete(#{
        path: p,
        summary: "No findings reported",
        confirmed: [],
        raw_count: 0,
    });
}

// Cap verification fan-out
const MAX_VERIFY = 12;
let to_verify = [];
let vi = 0;
for f in findings {
    if vi < MAX_VERIFY {
        to_verify.push(f);
    }
    vi += 1;
}
if findings.len() > MAX_VERIFY {
    log("Truncated verification to " + MAX_VERIFY.to_string() + " of " + findings.len().to_string() + " findings");
}

// ---- Phase: Verify ----
phase("Verify");
log("Adversarially verifying " + to_verify.len().to_string() + " findings");

let vjobs = [];
let fi = 0;
for f in to_verify {
    let issue_json = json_encode(f.issue);
    let file_json = json_encode(f.file);
    let cat_json = json_encode(f.category);
    let ev_json = json_encode(f.evidence);
    let vprompt = "";
    vprompt += "Adversarially VERIFY this audit finding. Default real=false unless you independently confirm.\n";
    vprompt += "Category: " + cat_json + "\n";
    vprompt += "File: " + file_json + "\n";
    vprompt += "Issue: " + issue_json + "\n";
    vprompt += "Claimed evidence: " + ev_json + "\n\n";
    vprompt += "You MUST open the cited file (and related code) with read_file/grep.\n";
    vprompt += "Set real=true ONLY if the issue is still true in the current code, with concrete evidence you inspected.\n";
    vprompt += "Set real=false if: already fixed, intentional deferred scope, wrong file, speculative, or cannot verify.\n";
    vprompt += "In evidence, quote specific lines/symbols you saw. reason must explain the verdict.\n";
    vjobs.push(#{
        prompt: vprompt,
        label: "verify:" + fi.to_string(),
        capability_mode: "read-only",
        output_schema: verdict_schema,
    });
    fi += 1;
}

let verdicts = parallel(vjobs);

let confirmed = [];
let rejected = [];
let idx = 0;
for v in verdicts {
    let f = to_verify[idx];
    if v != () && v.success && v.output.real == true
        && v.output.evidence != () && v.output.evidence != "" {
        let sev = f.severity;
        if v.output.severity_adjust != () && v.output.severity_adjust != "" {
            sev = v.output.severity_adjust;
        }
        confirmed.push(#{
            category: f.category,
            severity: sev,
            file: f.file,
            issue: f.issue,
            evidence: v.output.evidence,
            suggested_fix: f.suggested_fix,
            verify_reason: v.output.reason,
        });
    } else {
        let why = "unverified or failed";
        if v != () && v.success && v.output.reason != () {
            why = v.output.reason;
        }
        rejected.push(#{
            category: f.category,
            file: f.file,
            issue: f.issue,
            reason: why,
        });
    }
    idx += 1;
}

log(confirmed.len().to_string() + "/" + to_verify.len().to_string() + " findings confirmed");

// ---- Phase: Report ----
phase("Report");

let conf_json = json_encode(confirmed);
let rej_json = json_encode(rejected);
let rep_prompt = "";
rep_prompt += "Write a clear PROJECT AUDIT REPORT in markdown for the Launchpad repo.\n";
rep_prompt += "Use only the verified data below — do not invent new findings.\n";
rep_prompt += "You may briefly group and prioritize; keep every confirmed finding listed.\n\n";
rep_prompt += "Confirmed findings (JSON):\n" + conf_json + "\n\n";
rep_prompt += "Rejected/unverified (JSON, for an appendix):\n" + rej_json + "\n\n";
rep_prompt += "Structure:\n";
rep_prompt += "# Project audit report\n";
rep_prompt += "## Executive summary\n";
rep_prompt += "## Confirmed findings (by severity)\n";
rep_prompt += "## Recommended actions (ordered)\n";
rep_prompt += "## Appendix: rejected / unverified\n";
rep_prompt += "Return markdown in the markdown field, a one-line summary, and up to 10 priority_actions.\n";
if confirmed.len() == 0 {
    rep_prompt += "If confirmed is empty, say so clearly and note how many were rejected.\n";
}

let report_r = agent(rep_prompt, #{
    label: "synthesize-report",
    capability_mode: "read-only",
    output_schema: report_schema,
});

let report_md = "# Project audit report\n\n(synthesis failed)\n";
let summary = "synthesis failed";
let actions = [];
if report_r != () && report_r.success && report_r.output.markdown != () {
    report_md = report_r.output.markdown;
    if report_r.output.summary != () {
        summary = report_r.output.summary;
    }
    if report_r.output.priority_actions != () {
        actions = report_r.output.priority_actions;
    }
} else {
    // Fallback: hand-build a minimal report
    report_md = "# Project audit report\n\n";
    report_md += "Confirmed: " + confirmed.len().to_string() + "\n\n";
    for c in confirmed {
        report_md += "- **" + c.severity + "** [" + c.category + "] `" + c.file + "`: " + c.issue + "\n";
    }
    summary = confirmed.len().to_string() + " confirmed findings (fallback report)";
}

let report_path = write_scratch_file("report.md", report_md);
log("Report written to " + report_path);

// ---- Phase: Docs (optional) ----
if apply_docs {
    phase("Docs");
    let doc_findings = [];
    for c in confirmed {
        if c.category == "docs" {
            doc_findings.push(c);
        }
    }
    if doc_findings.len() == 0 {
        log("apply_docs=true but no confirmed docs findings — skipping");
        complete(#{
            path: report_path,
            summary: summary,
            confirmed: confirmed,
            rejected_count: rejected.len(),
            priority_actions: actions,
            docs_applied: false,
        });
    }

    await_user("user", "About to apply confirmed documentation fixes (" + doc_findings.len().to_string() + " items). Resume to proceed, or stop the workflow to leave docs unchanged.");

    let docs_json = json_encode(doc_findings);
    let fix_prompt = "";
    fix_prompt += "Apply ONLY the confirmed documentation fixes listed below for the Launchpad repo.\n";
    fix_prompt += "Rules:\n";
    fix_prompt += "- Edit markdown/yaml docs only (README.md, AGENTS.md, docs/**, .grok/skills/**). No Go source changes.\n";
    fix_prompt += "- Make minimal, accurate edits. Do not invent features.\n";
    fix_prompt += "- Prefer surgical patches over rewrites.\n";
    fix_prompt += "- After edits, re-read the changed sections to confirm consistency.\n\n";
    fix_prompt += "Confirmed docs findings (JSON):\n" + docs_json + "\n";

    let fix_r = agent(fix_prompt, #{
        label: "apply-docs",
        capability_mode: "read-write",
    });
    let docs_ok = fix_r != () && fix_r.success;
    log("Docs apply success=" + docs_ok.to_string());
    complete(#{
        path: report_path,
        summary: summary,
        confirmed: confirmed,
        rejected_count: rejected.len(),
        priority_actions: actions,
        docs_applied: docs_ok,
    });
}

complete(#{
    path: report_path,
    summary: summary,
    confirmed: confirmed,
    rejected_count: rejected.len(),
    priority_actions: actions,
    docs_applied: false,
    hint: "Re-run with args.apply_docs=true to patch confirmed documentation findings after a confirmation gate",
});
