import { useState } from "react";
import { Box, Button, Tooltip } from "@mui/material";
import { Check, Copy, X } from "@phosphor-icons/react";
import { copyText } from "../lib/clipboard";

const DEFAULT_LABELS = {
  copy: "Copy",
  copied: "Copied",
  copyFailed: "Failed",
  copiedLive: "Copied to clipboard",
  copyFailedLive: "Copy failed"
};

export function CopyButton({ text, labels = DEFAULT_LABELS, sx }) {
  const [state, setState] = useState("idle");
  const [timer, setTimer] = useState(null);

  const handleCopy = async () => {
    const ok = await copyText(text);
    setState(ok ? "copied" : "failed");
    if (timer) window.clearTimeout(timer);
    setTimer(window.setTimeout(() => setState("idle"), 1600));
  };

  const Icon = state === "copied" ? Check : state === "failed" ? X : Copy;
  const label = state === "copied" ? labels.copied : state === "failed" ? labels.copyFailed : labels.copy;

  return (
    <>
      <Tooltip title={label}>
        <Button
          size="small"
          variant="text"
          onClick={handleCopy}
          startIcon={<Icon size={14} />}
          aria-label={label}
          sx={[
            {
              minHeight: 26,
              px: 0.75,
              fontSize: "0.6875rem",
              fontWeight: 500,
              color: "text.secondary",
              whiteSpace: "nowrap",
              "&:hover": { color: "text.primary", bgcolor: "action.selected" }
            },
            ...(Array.isArray(sx) ? sx : [sx])
          ]}
        >
          {label}
        </Button>
      </Tooltip>
      <Box aria-live="polite" sx={{ position: "absolute", width: "1px", height: "1px", overflow: "hidden", clip: "rect(0 0 0 0)" }}>
        {state === "copied" ? labels.copiedLive : state === "failed" ? labels.copyFailedLive : ""}
      </Box>
    </>
  );
}
