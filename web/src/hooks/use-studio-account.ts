import { useQuery } from "@tanstack/react-query";
import { getStudioAccountSummary, studioConsoleNavigationURL } from "@/services/api/studio-account";
import { useUserStore } from "@/stores/use-user-store";

export function useStudioAccount(returnPath?: string) {
    const userId = useUserStore((state) => state.user?.id);
    const platformIdentity = useUserStore((state) => state.platformIdentity);
    const query = useQuery({
        queryKey: ["studio-account-summary", userId],
        queryFn: ({ signal }) => getStudioAccountSummary(signal),
        enabled: !!userId && platformIdentity,
        retry: false,
        gcTime: 0,
        // Session activation rechecks the identity before refreshing this cache.
        refetchOnWindowFocus: false,
    });
    // A Console account change can precede Studio's session update. A cached
    // response may only be shown in the account it was returned for.
    const account = !query.isError && query.data?.userId === userId ? query.data : undefined;
    const navigation =
        returnPath && account
            ? {
                  account: studioConsoleNavigationURL(account, "account", returnPath),
                  wallet: studioConsoleNavigationURL(account, "wallet", returnPath),
                  usage: studioConsoleNavigationURL(account, "usage", returnPath),
              }
            : undefined;
    return { account, query, navigation };
}
