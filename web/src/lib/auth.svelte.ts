import { getMe, type UserInfo } from '$lib/api';

interface AuthState {
	user: UserInfo | null;
	checked: boolean;
}

function createAuth() {
	let state = $state<AuthState>({ user: null, checked: false });

	async function check(): Promise<UserInfo | null> {
		try {
			const user = await getMe();
			state.user = user;
		} catch {
			state.user = null;
		}
		state.checked = true;
		return state.user;
	}

	function setUser(user: UserInfo | null) {
		state.user = user;
		state.checked = true;
	}

	function clear() {
		state.user = null;
	}

	return {
		get user() {
			return state.user;
		},
		get checked() {
			return state.checked;
		},
		check,
		setUser,
		clear
	};
}

export const auth = createAuth();
