const BASE = '/api/podgist/v1';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
	const res = await fetch(`${BASE}${path}`, {
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json', ...options?.headers },
		...options
	});
	if (!res.ok) {
		const text = await res.text().catch(() => res.statusText);
		throw new ApiError(res.status, text);
	}
	if (res.status === 204) return undefined as T;
	return res.json() as Promise<T>;
}

export class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
	}
}

// --- Auth ---

export interface UserInfo {
	username: string;
	created_at: string;
}

export async function register(username: string, password: string): Promise<UserInfo> {
	return request('/register', {
		method: 'POST',
		body: JSON.stringify({ username, password })
	});
}

export async function login(username: string, password: string): Promise<UserInfo> {
	return request('/login', {
		method: 'POST',
		body: JSON.stringify({ username, password })
	});
}

export async function logout(): Promise<void> {
	return request('/logout', { method: 'POST' });
}

export async function getMe(): Promise<UserInfo> {
	return request('/me');
}

// --- Dashboard ---

export interface EpisodeAction {
	podcast_url: string;
	podcast_title?: string;
	episode_url: string;
	episode_title?: string;
	action: string;
	timestamp: string;
	started?: number;
	position?: number;
	total?: number;
	device_uid?: string;
}

export interface DashboardData {
	subscription_count: number;
	device_count: number;
	episode_action_count: number;
	recent_actions: EpisodeAction[];
}

export async function getDashboard(): Promise<DashboardData> {
	return request('/dashboard');
}

// --- History ---

export interface PlaybackHistoryEntry {
	podcast_url: string;
	podcast_title?: string;
	episode_url: string;
	episode_title?: string;
	timestamp: string;
	position?: number;
	total?: number;
	device_uid?: string;
}

export async function getHistory(): Promise<PlaybackHistoryEntry[]> {
	return request('/history');
}

// --- Subscriptions ---

export interface Subscription {
	podcast_url: string;
	podcast_title?: string;
	devices: string[];
}

export async function getSubscriptions(): Promise<Subscription[]> {
	return request('/subscriptions');
}

// --- Devices ---

export interface Device {
	uid: string;
	caption: string;
	type: string;
	subscription_count: number;
	created_at: string;
	updated_at: string;
}

export async function getDevices(): Promise<Device[]> {
	return request('/devices');
}

// --- Account ---

export interface AccountData {
	username: string;
	created_at: string;
	session_expires_at: string;
}

export async function getAccount(): Promise<AccountData> {
	return request('/account');
}
