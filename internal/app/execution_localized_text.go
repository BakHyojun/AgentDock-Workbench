package app

import "github.com/uvwt/agentdock/internal/activity"

func describeOwnedManagement(event activity.Event) activity.Event {
	return activity.DescribeManagement(event)
}
