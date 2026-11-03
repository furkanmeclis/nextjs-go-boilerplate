import { realtimeConfig } from "@/config/realtime";

/**
 * Channel name builders — never hardcode channel strings in features.
 */
export const RealtimeChannels = {
  systemNotifications: () => realtimeConfig.channels.systemNotifications,
  organization: (orgUuid: string) =>
    realtimeConfig.channels.organization(orgUuid),
  user: (userId: number | string) => realtimeConfig.channels.user(userId),
  conversation: (conversationUuid: string) =>
    realtimeConfig.channels.conversation(conversationUuid),
} as const;

export type RealtimeChannelBuilder = typeof RealtimeChannels;

export function isAuthorizedChannelShape(channel: string): boolean {
  if (channel === realtimeConfig.channels.systemNotifications) return true;
  if (/^org:[0-9a-fA-F-]+$/.test(channel)) return true;
  if (/^user:[0-9a-fA-F-]+$/.test(channel)) return true;
  if (/^user:\d+$/.test(channel)) return true;
  if (/^conversation:[0-9a-fA-F-]+$/.test(channel)) return true;
  return false;
}
