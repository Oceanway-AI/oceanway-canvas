import { useQuery } from "@tanstack/react-query";
import { getStudioAccountSummary, studioAccountMatches, studioConsoleNavigationURL } from "@/services/api/studio-account";
import { useUserStore } from "@/stores/use-user-store";

export function useStudioAccount(returnPath?: string) {
    const userId = useUserStore((state) => state.user?.id);
    const platformIdentity = useUserStore((state) => state.platformIdentity);
    const context = useUserStore((state) => state.workspaceContext);
    const query = useQuery({
        queryKey: ["studio-account-summary", userId, context?.workspaceId, context?.membershipGeneration],
        queryFn: ({ signal }) => getStudioAccountSummary(signal),
        enabled: !!userId && platformIdentity && context?.userId === userId,
        retry: false,
        gcTime: 0,
        // Session activation rechecks the identity before refreshing this cache.
        refetchOnWindowFocus: false,
    });
    // A Console account change can precede Studio's session update. A cached
    // response may only be shown in the account it was returned for.
    const account = !query.isError && studioAccountMatches(query.data, userId, context?.workspaceId, context?.membershipGeneration) ? query.data : undefined;
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
