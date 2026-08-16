"use client";

import { useEffect, useRef } from "react";

export function useInfiniteScroll(hasMore: boolean, loading: boolean, onLoadMore: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = ref.current;
    if (!node || !hasMore || loading) return;
    const observer = new IntersectionObserver(entries => {
      if (entries.some(entry => entry.isIntersecting)) onLoadMore();
    }, { rootMargin: "500px 0px" });
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasMore, loading, onLoadMore]);
  return ref;
}
