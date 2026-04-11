import pandas as pd

# ── Config ──────────────────────────────────────────────────────────────────
CSV_PATH       = "DataExtensionStorage_526001350_2026-04-09.csv"
TOTAL_ALLOC_GB = 45.0               # 45 GB

# ── Load data ────────────────────────────────────────────────────────────────
df = pd.read_csv(CSV_PATH)
df["BusinessUnitName"] = df["BusinessUnitName"].str.strip()

# ── Clean UsedGB: coerce 'Data_Not_Available' (and any non-numeric) to 0 ────
invalid_mask  = pd.to_numeric(df["UsedGB"], errors="coerce").isna()
invalid_count = invalid_mask.sum()
if invalid_count > 0:
    print(f"  ⚠  {invalid_count} row(s) had non-numeric UsedGB "
          f"(e.g. 'Data_Not_Available') — treated as 0 GB.\n")
df["UsedGB"] = pd.to_numeric(df["UsedGB"], errors="coerce").fillna(0.0)
df["RecordCount"] = pd.to_numeric(df["RecordCount"], errors="coerce").fillna(0).astype(int)

# ── Group by MID ─────────────────────────────────────────────────────────────
summary = (
    df.groupby(["MID", "BusinessUnitName"])
    .agg(
        DataExtensions  = ("DataExtensionName", "count"),
        UsedGB          = ("UsedGB", "sum"),
        TotalRecords    = ("RecordCount", "sum"),
    )
    .reset_index()
)

summary["UsedTB"]          = summary["UsedGB"] / 1024
summary["PctOfTotalAlloc"] = summary["UsedGB"] / TOTAL_ALLOC_GB * 100

# ── Overall totals ───────────────────────────────────────────────────────────
total_used_gb  = summary["UsedGB"].sum()
total_used_tb  = total_used_gb / 1024
remaining_gb   = TOTAL_ALLOC_GB - total_used_gb
remaining_tb   = remaining_gb / 1024
pct_used       = total_used_gb / TOTAL_ALLOC_GB * 100
pct_free       = 100 - pct_used

# ── Print report ─────────────────────────────────────────────────────────────
SEP  = "=" * 72
sep  = "-" * 72

print(SEP)
print("  STORAGE USAGE REPORT  —  per MID")
print(f"  Total allocated space : {TOTAL_ALLOC_GB:,.2f} GB")
print(SEP)

for _, row in summary.iterrows():
    print(f"\n  MID              : {row['MID']}")
    print(f"  Business Unit    : {row['BusinessUnitName']}")
    print(f"  Data Extensions  : {row['DataExtensions']:,}")
    print(f"  Total Records    : {row['TotalRecords']:,}")
    print(f"  Used Storage     : {row['UsedGB']:>10.5f} GB  "
          f"({row['UsedTB']:.6f} TB)  "
          f"[{row['PctOfTotalAlloc']:.4f}% of total 45 GB]")
    print(sep)

print(f"\n  ── COMBINED TOTALS ──")
print(f"  Total Used       : {total_used_gb:>10.5f} GB  ({total_used_tb:.6f} TB)  [{pct_used:.4f}%]")
print(f"  Remaining Free   : {remaining_gb:>10.2f} GB  ({remaining_tb:.4f} TB)       [{pct_free:.4f}%]")
print(SEP)

# ── Optional: export to CSV ──────────────────────────────────────────────────
out_cols = ["MID", "BusinessUnitName", "DataExtensions",
            "TotalRecords", "UsedGB", "UsedTB", "PctOfTotalAlloc"]

summary[out_cols].to_csv("storage_summary_by_mid.csv", index=False)
print("\n  Summary exported → storage_summary_by_mid.csv")