{
  "task_id": "20260620-195200-fix-audit-issues",
  "goal": "Fix all 33 issues from the audit report (debug/审查报告.md) across 9 modules",
  "scope": "All non-security, non-performance, non-mobile features marked as 'completed'",
  "non_goals": [
    "New feature development",
    "Security/Performance/Mobile API improvements",
    "Refactoring architecture"
  ],
  "allowed_operations": [
    "Edit backend Python files",
    "Edit frontend TypeScript/TSX files",
    "Verify with code reading",
    "Write tests to verify fixes"
  ],
  "success_criteria": [
    "debug/审查报告.md has all 33 issues marked as FIXED with evidence",
    "No regression introduced"
  ],
  "verification_gates": [
    "Each fix verified by reading the patched code",
    "Integration points checked: frontend calls match backend routes"
  ],
  "stale_count": 0
}
