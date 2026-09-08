-- 0012_missed_medication_alerts — alert the care circle when a pending dose
-- has remained unrecorded beyond the medication domain's two-hour grace period.

ALTER TABLE notification_preferences
    ADD COLUMN missed_medication_alerts boolean NOT NULL DEFAULT true;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_type_recognised,
    ADD CONSTRAINT notifications_type_recognised
        CHECK (notification_type IN (
            'MEDICATION_REMINDER',
            'MEDICATION_MISSED',
            'APPOINTMENT_REMINDER',
            'TASK_REMINDER',
            'TASK_OVERDUE',
            'CARE_ACTIVITY'
        ));
