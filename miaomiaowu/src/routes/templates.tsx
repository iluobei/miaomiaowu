// @ts-nocheck
import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/templates')({
	component: TemplatesLayout,
})

function TemplatesLayout() {
	return <Outlet />
}
