// ADM gate: post-implementation dual review (spec compliance + quality).
// Optional args:
//   target      — branch, PR URL, or git range (default: working tree / HEAD vs main)
//   spec_path   — design spec for compliance (optional)
//   plan_path   — implementation plan (optional)
//   base        — merge base ref for diff (default origin/main)
let meta = #{
    name: "adm-review",
    description: "ADM dual review: spec compliance + quality with adversarial verification",
    when_to_use: "After an ADM implementer task or before opening a feature PR",
    phases: [
        #{ title: "Review", detail: "parallel spec-compliance and quality" },
        #{ title: "Verify", detail: "adversarial check of findings" },
        #{ title: "Report", detail: "approve vs request-changes" },
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
                "required": ["severity", "file", "issue", "evidence"],
                "properties": #{
                    "severity": #{ "type": "string", "enum": ["blocker", "major", "minor"] },
                    "file": #{ "type": "string" },
                    "issue": #{ "type": "string" },
                    "evidence": #{ "type": "string" },
                    "category": #{ "type": "string" },
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
    "required": ["approve", "markdown", "summary"],
    "properties": #{
        "approve": #{ "type": "boolean" },
        "markdown": #{ "type": "string" },
        "summary": #{ "type": "string" },
        "must_fix": #{
            "type": "array",
            "maxItems": 12,
            "items": #{ "type": "string" },
        },
    },
};

let target = "HEAD";
let spec_path = ();
let plan_path = ();
let base = "origin/main";
if args != () {
    if args.target != () { target = args.target; }
    if args.spec_path != () { spec_path = args.spec_path; }
    if args.plan_path != () { plan_path = args.plan_path; }
    if args.base != () { base = args.base; }
}

let spec_note = "none";
if spec_path != () { spec_note = spec_path; }
let plan_note = "none";
if plan_path != () { plan_note = plan_path; }

// ---- Phase: Review ----
phase("Review");
log("Dual review target=" + target + " base=" + base);

let jobs = [];

// Spec compliance
let sc = "";
sc += "You are an ADM SPEC-COMPLIANCE reviewer for Launchpad.\n";
sc += "Use read-only tools. Inspect the actual diff and code — do not answer from memory.\n\n";
sc += "Target: " + target + "\n";
sc += "Diff base: " + base + "\n";
sc += "Run: git fetch if needed; git diff " + base + "..." + target + "\n";
sc += "Do NOT use git diff base...HEAD unless target is literally HEAD — the session checkout is often main.\n";
sc += "Spec path: " + spec_note + " (read this path as given; .worktrees/ prefixes are real files).\n";
sc += "Plan path: " + plan_note + "\n\n";
sc += "Check:\n";
sc += "1. Implementation matches the recommended approach in the spec (if present).\n";
sc += "2. No drive-by features beyond the plan slice.\n";
sc += "3. DOMAIN/OpenAPI/docs updates present when entities or routes changed.\n";
sc += "4. Tests cover the stated strategy; deploy-path changes need e2e or clear justification.\n";
sc += "5. No App model /v1/apps regressions; no deferred-boundary half-builds (AGENTS.md).\n\n";
sc += "If no spec_path, judge against plan_path + DOMAIN.md + AGENTS.md MVP only.\n";
sc += "Report at most 8 findings. category should be 'spec'. severity blocker|major|minor.\n";
sc += "Empty findings valid ONLY after reading the diff.\n";
jobs.push(#{
    prompt: sc,
    label: "review:spec",
    capability_mode: "read-only",
    output_schema: findings_schema,
});

// Quality
let qc = "";
qc += "You are an ADM CODE QUALITY reviewer for Launchpad.\n";
qc += "Use read-only tools. Inspect the actual diff and surrounding code.\n\n";
qc += "Target: " + target + "\n";
qc += "Diff base: " + base + "\n";
qc += "Run: git diff " + base + "..." + target + " (not base...HEAD unless target is HEAD).\n\n";
qc += "Check against repo conventions (AGENTS.md):\n";
qc += "1. Domain free of SQL/HTTP; store uses ? placeholders; multi-step writes use Transact.\n";
qc += "2. Errors use pkg/launchpad sentinels; API problem+json where relevant.\n";
qc += "3. Tests meaningful (not tautologies); mise-friendly verify paths.\n";
qc += "4. No secrets in logs; sensitivity/redaction respected for config.\n";
qc += "5. Clear naming; no dead stubs left mid-feature; focused commits if visible.\n";
qc += "6. CLI ambient env / X-Launchpad-Environment consistency when CLI touched.\n\n";
qc += "Report at most 8 findings. category 'quality'. severity blocker|major|minor.\n";
qc += "Empty findings valid ONLY after reading the code.\n";
jobs.push(#{
    prompt: qc,
    label: "review:quality",
    capability_mode: "read-only",
    output_schema: findings_schema,
});

let results = parallel(jobs);

let findings = [];
let si = 0;
for r in results {
    if r == () || !r.success {
        log("review slot " + si.to_string() + " failed");
        findings.push(#{
            severity: "blocker",
            file: "(reviewer)",
            issue: "Review agent failed — treat as request-changes until re-run",
            evidence: "parallel slot " + si.to_string(),
            category: "process",
        });
    } else if r.output.findings != () {
        for f in r.output.findings {
            if f != () && f.issue != () {
                let sev = if f.severity != () { f.severity } else { "minor" };
                let file = if f.file != () { f.file } else { "(unspecified)" };
                let ev = if f.evidence != () { f.evidence } else { "" };
                let cat = if f.category != () { f.category } else { "review" };
                findings.push(#{
                    severity: sev,
                    file: file,
                    issue: f.issue,
                    evidence: ev,
                    category: cat,
                });
            }
        }
    }
    si += 1;
}

log("Raw findings: " + findings.len().to_string());

if findings.len() == 0 {
    let md = "# ADM dual review\n\n**Approve:** yes\n\nNo findings from reviewers (or empty after read).\n";
    md += "Target: " + target + " base: " + base + "\n";
    let p = write_scratch_file("report.md", md);
    complete(#{
        path: p,
        approve: true,
        summary: "No findings — approve",
        confirmed: [],
        must_fix: [],
    });
}

// ---- Phase: Verify ----
phase("Verify");
const MAX_V = 12;
let to_verify = [];
let n = 0;
for f in findings {
    if n < MAX_V { to_verify.push(f); }
    n += 1;
}
if findings.len() > MAX_V {
    log("Truncated verification to " + MAX_V.to_string());
}

let vjobs = [];
let vi = 0;
for f in to_verify {
    let vp = "";
    vp += "Adversarially verify this review finding. Default real=false.\n";
    vp += "Target/base: " + target + " / " + base + "\n";
    vp += "file: " + json_encode(f.file) + "\n";
    vp += "severity: " + json_encode(f.severity) + "\n";
    vp += "issue: " + json_encode(f.issue) + "\n";
    vp += "claimed evidence: " + json_encode(f.evidence) + "\n\n";
    vp += "Read the cited file and surrounding diff. real=true only with independent evidence.\n";
    vjobs.push(#{
        prompt: vp,
        label: "verify:" + vi.to_string(),
        capability_mode: "read-only",
        output_schema: verdict_schema,
    });
    vi += 1;
}

let verdicts = parallel(vjobs);
let confirmed = [];
let rejected = [];
let idx = 0;
for v in verdicts {
    let f = to_verify[idx];
    if v != () && v.success && v.output.real == true
        && v.output.evidence != () && v.output.evidence != "" {
        confirmed.push(#{
            severity: f.severity,
            file: f.file,
            issue: f.issue,
            evidence: v.output.evidence,
            category: f.category,
            reason: v.output.reason,
        });
    } else {
        let why = "unverified";
        if v != () && v.success && v.output.reason != () { why = v.output.reason; }
        rejected.push(#{ file: f.file, issue: f.issue, reason: why });
    }
    idx += 1;
}

log("Confirmed " + confirmed.len().to_string() + "/" + to_verify.len().to_string());

// ---- Phase: Report ----
phase("Report");

let approve = true;
let must_fix = [];
for c in confirmed {
    if c.severity == "blocker" || c.severity == "major" {
        approve = false;
        must_fix.push("[" + c.severity + "] " + c.file + ": " + c.issue);
    }
}

let conf_json = json_encode(confirmed);
let rej_json = json_encode(rejected);
let rp = "";
rp += "Write an ADM DUAL REVIEW report for the Launchpad orchestrator.\n";
rp += "Target: " + target + " base: " + base + "\n";
rp += "Spec: " + spec_note + " plan: " + plan_note + "\n\n";
rp += "Confirmed findings (JSON):\n" + conf_json + "\n\n";
rp += "Rejected (JSON):\n" + rej_json + "\n\n";
rp += "Rules: approve=false if any confirmed blocker or major; minors alone may still approve with notes.\n";
rp += "Structure: # ADM dual review; Approve yes/no; Must-fix; Nits; Appendix rejected.\n";
rp += "Do not invent findings.\n";

let report_r = agent(rp, #{
    label: "review-report",
    capability_mode: "read-only",
    output_schema: report_schema,
});

let markdown = "# ADM dual review\n\n";
let summary = if approve { "Approve with notes" } else { "Request changes" };
if report_r != () && report_r.success && report_r.output.markdown != () {
    markdown = report_r.output.markdown;
    if report_r.output.summary != () { summary = report_r.output.summary; }
    if report_r.output.approve == false { approve = false; }
    if report_r.output.must_fix != () {
        for m in report_r.output.must_fix {
            if m != () && m != "" { must_fix.push(m); }
        }
    }
} else {
    markdown += "**Approve:** " + approve.to_string() + "\n\n";
    for c in confirmed {
        markdown += "- **" + c.severity + "** `" + c.file + "`: " + c.issue + "\n";
    }
}

let path = write_scratch_file("report.md", markdown);
log("approve=" + approve.to_string() + " path=" + path);

complete(#{
    path: path,
    approve: approve,
    summary: summary,
    confirmed: confirmed,
    must_fix: must_fix,
    rejected_count: rejected.len(),
    hint: "Orchestrator: if approve=false, send must_fix back to implementer; do not open PR until green.",
});
