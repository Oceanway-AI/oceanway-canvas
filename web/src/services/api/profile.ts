import type { LocalUser } from "./auth";
import { http } from "./request";

export type UserProfile = {
    provider: "platform";
    consoleURL: string;
    user: LocalUser;
};

export const getUserProfile = (signal?: AbortSignal) => http.get<UserProfile>("/auth/profile", { signal });
