import { Box, Skeleton } from "@mui/material";
import { useReducedMotion } from "motion/react";

export function DocSkeleton({ copy }) {
  const reduced = useReducedMotion();
  const animation = reduced ? false : "wave";

  return (
    <Box aria-label={copy.skeleton.aria} aria-busy="true" sx={{ maxWidth: 760 }}>
      <Skeleton variant="text" width="22%" height={16} animation={animation} />
      <Skeleton variant="text" width="58%" height={48} sx={{ mt: 1.5 }} animation={animation} />
      <Skeleton variant="text" width="34%" height={18} sx={{ mt: 1 }} animation={animation} />
      <Skeleton variant="text" width="88%" height={22} sx={{ mt: 3 }} animation={animation} />
      <Skeleton variant="text" width="76%" height={22} animation={animation} />
      <Skeleton variant="rectangular" height={86} sx={{ mt: 3, borderRadius: 1 }} animation={animation} />
      <Skeleton variant="text" width="42%" height={30} sx={{ mt: 4 }} animation={animation} />
      <Skeleton variant="text" width="92%" height={20} animation={animation} />
      <Skeleton variant="text" width="84%" height={20} animation={animation} />
      <Skeleton variant="rectangular" height={132} sx={{ mt: 2.5, borderRadius: 1.25 }} animation={animation} />
      <Skeleton variant="text" width="90%" height={20} sx={{ mt: 3 }} animation={animation} />
      <Skeleton variant="text" width="72%" height={20} animation={animation} />
    </Box>
  );
}
