import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/custom-rules')({
	component: CustomRulesLayout,
})

function CustomRulesLayout() {
	return (
		<div className='min-h-svh bg-background'>
			<Outlet />
		</div>
	)
}
