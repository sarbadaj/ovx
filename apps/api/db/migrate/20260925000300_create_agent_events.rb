class CreateAgentEvents < ActiveRecord::Migration[8.0]
  def change
    create_table :agent_events do |t|
      t.references :task, null: false, foreign_key: true
      t.string :event_type, null: false
      t.jsonb :payload, null: false, default: {}
      t.timestamps
    end
    add_index :agent_events, [:task_id, :id]
  end
end
