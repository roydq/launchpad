// ADM gate: adversarial self-review of a design spec before implementation.
// Parent orchestrator keeps hard-stop authority; this workflow only reports pass/fail.
// Required args: { "spec_path": "docs/superpowers/specs/....md" }
// Optional: plan_path, domain_path (default docs/DOMAIN.md)
let meta = #{
    name: "adm-spec-review",
    description: "ADM self-approve gate: checklist + adversarial review of a design spec",
    when_to_use: "After drafting a spec under ADM, before self-approve / implementation",
    phases: [
        #{ title: "Review", detail: "parallel checklist dimensions" },
        #{ title: "Verify", detail: "adversarial check of blockers" },
        #{ title: "Verdict", detail: "pass/fail report for orchestrator" },
    ],
};

let findings_schema = #{
    "type": "object",
    "required": ["findings", "dimension_pass"],
    "properties": #{
        "dimension_pass": #{ "type": "boolean" },
        "findings": #{
            "type": "array",
            "maxItems": 6,
            "items": #{
                "type": "object",
                "required": ["check_id", "severity", "issue", "evidence"],
                "properties": #{
                    "check_id": #{ "type": "string" },
                    "severity": #{ "type": "string", "enum": ["blocker", "warning"] },
                    "issue": #{ "type": "string" },
                    "evidence": #{ "type": "string" },
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
    },
};

let report_schema = #{
    "type": "object",
    "required": ["pass", "markdown", "summary"],
    "properties": #{
        "pass": #{ "type": "boolean" },
        "markdown": #{ "type": "string" },
        "summary": #{ "type": "string" },
        "blockers": #{
            "type": "array",
            "maxItems": 12,
            "items": #{ "type": "string" },
        },
    },
};

let spec_path = ();
let plan_path = ();
let domain_path = "docs/DOMAIN.md";
if args != () {
    if args.spec_path != () { spec_path = args.spec_path; }
    if args.plan_path != () { plan_path = args.plan_path; }
    if args.domain_path != () { domain_path = args.domain_path; }
}
if spec_path == () {
    pause("verification", "Pass args.spec_path — path to the design spec markdown to review.");
}

let plan_note = "none";
if plan_path != () { plan_note = plan_path; }

// ---- Phase: Review (checklist dimensions) ----
phase("Review");
log("Reviewing spec " + spec_path + " (plan=" + plan_note + ")");

let dimensions = [
    #{
        id: "placeholders",
        title: "no placeholders / vague requirements",
        detail: "Fail if TBD/TODO, 'handle errors appropriately', or missing concrete acceptance criteria.",
    },
    #{
        id: "consistency",
        title: "internal consistency",
        detail: "API sketch, schema/migrations, CLI, and success criteria must agree. Flag contradictions.",
    },
    #{
        id: "scope",
        title: "single plan scope + recommended path",
        detail: "One vertical shippable slice; rejected approaches listed. Fail multi-service+secrets+OIDC mega-specs.",
    },
    #{
        id: "domain_mvp",
        title: "DOMAIN + MVP boundary",
        detail: "No contradiction of docs/DOMAIN.md or AGENTS.md deferred items without an explicit DOMAIN update in the plan.",
    },
    #{
        id: "tests_dod",
        title: "test strategy + DoD",
        detail: "Unit strategy present; deploy-path needs e2e-stub/smoke; DoD/acceptance criteria checkable without chat history.",
    },
];

let jobs = [];
for d in dimensions {
    let prompt = "";
    prompt += "You are an ADM SPEC SELF-REVIEWER for Launchpad.\n";
    prompt += "Read the REAL files with read_file/grep — do not invent contents.\n\n";
    prompt += "Spec path: " + spec_path + "\n";
    prompt += "Plan path (optional): " + plan_note + "\n";
    prompt += "Domain path: " + domain_path + "\n";
    prompt += "Also skim AGENTS.md MVP deferred list when judging boundaries.\n\n";
    prompt += "Checklist dimension: " + d.id + " — " + d.title + "\n";
    prompt += d.detail + "\n\n";
    prompt += "Canonical checklist is docs/AUTONOMOUS-MODE.md § Spec self-review (self-approve gate).\n";
    prompt += "Set dimension_pass=true only if this dimension is solid.\n";
    prompt += "Report at most 6 findings. severity blocker = must hard-stop self-approve; warning = fix preferred.\n";
    prompt += "check_id should be short (e.g. placeholders, routes-mismatch).\n";
    prompt += "Empty findings + dimension_pass=true is valid ONLY after reading the spec.\n";
    jobs.push(#{
        prompt: prompt,
        label: "spec:" + d.id,
        capability_mode: "read-only",
        output_schema: findings_schema,
    });
}

let results = parallel(jobs);

let findings = [];
let dim_fails = 0;
let di = 0;
for r in results {
    if r == () || !r.success {
        log("dimension slot " + di.to_string() + " failed");
        dim_fails += 1;
        findings.push(#{
            check_id: "agent-fail",
            severity: "blocker",
            issue: "Reviewer agent failed for a checklist dimension",
            evidence: "parallel slot " + di.to_string(),
        });
    } else {
        if r.output.dimension_pass == false {
            dim_fails += 1;
        }
        if r.output.findings != () {
            for f in r.output.findings {
                if f != () && f.issue != () {
                    let sev = if f.severity != () { f.severity } else { "warning" };
                    let cid = if f.check_id != () { f.check_id } else { "unspecified" };
                    let ev = if f.evidence != () { f.evidence } else { "" };
                    findings.push(#{
                        check_id: cid,
                        severity: sev,
                        issue: f.issue,
                        evidence: ev,
                    });
                }
            }
        }
    }
    di += 1;
}

log("Raw findings: " + findings.len().to_string() + "; dimension fail signals: " + dim_fails.to_string());

// ---- Phase: Verify blockers ----
phase("Verify");

let to_verify = [];
let max_v = 10;
let fi = 0;
for f in findings {
    if fi < max_v {
        // Prefer verifying blockers; still verify warnings for truth
        to_verify.push(f);
    }
    fi += 1;
}
if findings.len() > max_v {
    log("Truncated verification to " + max_v.to_string());
}

let confirmed = [];
let rejected = [];

if to_verify.len() > 0 {
    let vjobs = [];
    let vi = 0;
    for f in to_verify {
        let vp = "";
        vp += "Adversarially verify this SPEC review finding. Default real=false.\n";
        vp += "Spec: " + spec_path + "\n";
        vp += "check_id: " + json_encode(f.check_id) + "\n";
        vp += "severity: " + json_encode(f.severity) + "\n";
        vp += "issue: " + json_encode(f.issue) + "\n";
        vp += "claimed evidence: " + json_encode(f.evidence) + "\n\n";
        vp += "Open the spec (and DOMAIN/AGENTS if needed). real=true only with concrete quotes you inspected.\n";
        vjobs.push(#{
            prompt: vp,
            label: "verify:" + vi.to_string(),
            capability_mode: "read-only",
            output_schema: verdict_schema,
        });
        vi += 1;
    }
    let verdicts = parallel(vjobs);
    let idx = 0;
    for v in verdicts {
        let f = to_verify[idx];
        if v != () && v.success && v.output.real == true
            && v.output.evidence != () && v.output.evidence != "" {
            confirmed.push(#{
                check_id: f.check_id,
                severity: f.severity,
                issue: f.issue,
                evidence: v.output.evidence,
                reason: v.output.reason,
            });
        } else {
            let why = "unverified";
            if v != () && v.success && v.output.reason != () { why = v.output.reason; }
            rejected.push(#{ check_id: f.check_id, issue: f.issue, reason: why });
        }
        idx += 1;
    }
}

log("Confirmed " + confirmed.len().to_string() + " / verified " + to_verify.len().to_string());

// ---- Phase: Verdict ----
phase("Verdict");

let conf_json = json_encode(confirmed);
let rej_json = json_encode(rejected);
let rep = "";
rep += "Produce an ADM SPEC SELF-REVIEW verdict for the Launchpad orchestrator.\n";
rep += "Spec: " + spec_path + "\nPlan: " + plan_note + "\n\n";
rep += "Confirmed findings (JSON):\n" + conf_json + "\n\n";
rep += "Rejected/unverified (JSON):\n" + rej_json + "\n\n";
rep += "Rules:\n";
rep += "- pass=false if any confirmed finding has severity blocker, OR if review agents collapsed and nothing could be verified while dim_fails>0.\n";
rep += "- pass=true only when there are zero confirmed blockers (warnings may remain as notes).\n";
rep += "- markdown: short report with Executive summary, Blockers, Warnings, Recommendation (self-approve vs hard stop).\n";
rep += "- Do not invent findings beyond the JSON.\n";
rep += "dim_fails signal (agent/dimension soft fails): " + dim_fails.to_string() + "\n";

let report_r = agent(rep, #{
    label: "spec-verdict",
    capability_mode: "read-only",
    output_schema: report_schema,
});

let pass = true;
let summary = "spec review complete";
let markdown = "# ADM spec review\n\n";
let blockers = [];

// Fail closed: any confirmed blocker fails
for c in confirmed {
    if c.severity == "blocker" {
        pass = false;
        blockers.push(c.issue);
    }
}
// If no findings at all but multiple agent failures, fail closed
if confirmed.len() == 0 && findings.len() == 0 && dim_fails > 0 {
    pass = false;
    blockers.push("Checklist agents failed; cannot self-approve");
}

if report_r != () && report_r.success && report_r.output.markdown != () {
    markdown = report_r.output.markdown;
    if report_r.output.summary != () { summary = report_r.output.summary; }
    // Trust synthesizer pass only if it does not contradict blockers
    if report_r.output.pass == false { pass = false; }
    if report_r.output.blockers != () {
        for b in report_r.output.blockers {
            if b != () && b != "" { blockers.push(b); }
        }
    }
} else {
    markdown += "pass=" + pass.to_string() + "\n\n## Confirmed\n";
    for c in confirmed {
        markdown += "- **" + c.severity + "** [" + c.check_id + "] " + c.issue + "\n";
    }
    summary = if pass { "Self-approve OK (no confirmed blockers)" } else { "Hard stop: confirmed blockers" };
}

let path = write_scratch_file("report.md", markdown);
log("Verdict pass=" + pass.to_string() + " path=" + path);

complete(#{
    path: path,
    pass: pass,
    summary: summary,
    blockers: blockers,
    confirmed: confirmed,
    rejected_count: rejected.len(),
    hint: "Orchestrator: if pass=false, hard-stop ADM design gate; do not self-approve.",
});
