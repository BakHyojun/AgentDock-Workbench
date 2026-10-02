using System.Windows;
using System.Windows.Automation;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Threading;
using AgentDock.ControlPanel;

internal static class ActivityResetDialogTests
{
    internal static void Run(Action<bool, string> check)
    {
        foreach (var culture in new[] { "en-US", "ko-KR", "zh-CN" })
        foreach (var font in new[] { 12d, 20d })
        {
            UiText.ApplyPreference(culture);
            var owner = new Window { Width = 600, Height = 400, FontSize = font, ShowInTaskbar = false, Left = -10000, Top = -10000 };
            owner.Show();
            Exception? failure = null;
            var observed = false;
            var timer = new DispatcherTimer { Interval = TimeSpan.FromMilliseconds(20) };
            timer.Tick += (_, _) =>
            {
                var dialog = System.Windows.Application.Current.Windows.Cast<Window>().FirstOrDefault(window => window.Owner == owner);
                if (dialog is null) return;
                timer.Stop();
                try
                {
                    dialog.UpdateLayout();
                    var scroller = Descendants(dialog).OfType<ScrollViewer>().Single();
                    var label = (TextBlock)scroller.Content;
                    check(label.Text == UiText.Get("ExecutionActivityResetWarning"), "Reset confirmation lost its scope warning.");
                    check(scroller.ViewportHeight > 0 && dialog.ActualHeight <= SystemParameters.WorkArea.Height + 1, "Reset confirmation is not bounded by the screen.");
                    check(label.ActualHeight >= label.DesiredSize.Height - 1, "Reset scope text is clipped inside its scroll container.");
                    if (scroller.ScrollableHeight > 1)
                    {
                        scroller.ScrollToEnd(); dialog.UpdateLayout();
                        check(scroller.VerticalOffset > 0, "Reset scope cannot be read to the end.");
                    }
                    var buttons = Descendants(dialog).OfType<Button>().ToArray();
                    var cancel = buttons.Single(button => AutomationProperties.GetAutomationId(button) == "ExecutionConfirmCancel");
                    var accept = buttons.Single(button => AutomationProperties.GetAutomationId(button) == "ExecutionConfirmAccept");
                    check(cancel.IsCancel && !accept.IsDefault, "Destructive confirmation defaults to acceptance.");
                    observed = true;
                    cancel.RaiseEvent(new RoutedEventArgs(Button.ClickEvent, cancel));
                }
                catch (Exception ex) { failure = ex; dialog.Close(); }
            };
            try
            {
                timer.Start();
                var accepted = ExecutionDialogs.Confirm(owner, UiText.Get("ExecutionActivityResetTitle"),
                    UiText.Get("ExecutionActivityResetWarning"), UiText.Get("ExecutionActivityResetAccept"), scrollExplanation: true);
                if (failure is not null) throw failure;
                check(observed && !accepted, "Cancelling reset confirmation did not preserve history.");
            }
            finally { timer.Stop(); owner.Close(); }
        }
        UiText.ApplyPreference("ko-KR");
    }

    private static IEnumerable<DependencyObject> Descendants(DependencyObject parent)
    {
        for (var index = 0; index < VisualTreeHelper.GetChildrenCount(parent); index++)
        {
            var child = VisualTreeHelper.GetChild(parent, index);
            yield return child;
            foreach (var nested in Descendants(child)) yield return nested;
        }
    }
}
