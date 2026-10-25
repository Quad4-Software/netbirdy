import { usePathname, useSearchParams } from "next/navigation";
import { useEffect } from "react";

export const NavigationEvents = () => {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const analytics = undefined as any;

  useEffect(() => {
    //const url = `${pathname}?${searchParams}`;
    if (analytics && analytics.initialized) analytics.trackPageView();
  }, [pathname, searchParams]);

  return null;
};
